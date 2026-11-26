// Package appstore handles App Store Connect polling, webhooks, and webhook automation.
package appstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "appstore" }
func (Plugin) Version() string { return "1.1.0" }

// Apple signs the raw body: X-Apple-Signature: hmacsha256=<hex>, key = the secret set on the webhook.
func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "hmacsha256=" + hex.EncodeToString(mac.Sum(nil))
	if secret == "" || !hmac.Equal([]byte(r.Header.Get("X-Apple-Signature")), []byte(want)) {
		return errors.New("bad signature")
	}
	return nil
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Data struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				NewValue      string `json:"newValue"`
				AppVersionID  string `json:"appVersionId"`
				VersionString string `json:"versionString"`
				State         string `json:"state"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	state := p.Data.Attributes.NewValue
	if state == "" {
		state = p.Data.Attributes.State
	}
	ver := p.Data.Attributes.VersionString
	if ver == "" {
		ver = p.Data.Attributes.AppVersionID
	}
	extID := p.Data.Attributes.AppVersionID
	if extID == "" {
		extID = p.Data.ID
	}
	stage, prog := stateProgress(state)
	return &core.DeploymentEvent{
		Platform:   "appstore",
		Status:     mapState(state),
		Version:    ver,
		ExternalID: extID,
		Stage:      stage,
		Progress:   prog,
		Message:    strings.TrimSpace(p.Data.Type + ": " + state),
	}, nil
}

func mapState(state string) string {
	switch {
	case state == "READY_FOR_SALE" || state == "READY_FOR_DISTRIBUTION" || state == "VALID":
		return core.StatusSuccess
	case strings.Contains(state, "REJECTED") || state == "INVALID_BINARY" || state == "FAILED":
		return core.StatusFailed
	}
	return core.StatusInProgress
}

func stateProgress(state string) (string, int) {
	switch state {
	case "READY_FOR_SALE", "READY_FOR_DISTRIBUTION":
		return "live", 100
	case "PENDING_DEVELOPER_RELEASE", "PENDING_APPLE_RELEASE":
		return "approved", 85
	case "IN_REVIEW":
		return "in_review", 65
	case "WAITING_FOR_REVIEW", "WAITING_FOR_EXPORT_COMPLIANCE":
		return "waiting_for_review", 40
	case "PROCESSING", "PROCESSING_FOR_APP_STORE":
		return "processing", 25
	case "PREPARE_FOR_SUBMISSION":
		return "preparing", 15
	default:
		if strings.Contains(state, "REJECTED") || state == "INVALID_BINARY" || state == "FAILED" {
			return strings.ToLower(state), 100
		}
		return strings.ToLower(state), 50
	}
}

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "issuer_id", Label: "Issuer ID"},
		{Key: "key_id", Label: "Key ID"},
		{Key: "private_key", Label: "Private key (.p8 contents)", Secret: true, Multiline: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "app_id", Label: "App ID (numeric)"},
	}
}

var baseURL = "https://api.appstoreconnect.apple.com"

// token builds an ES256 JWT (valid 10 min) as App Store Connect requires.
func token(issuer, keyID, pemKey string) (string, error) {
	blk, _ := pem.Decode([]byte(pemKey))
	if blk == nil {
		return "", errors.New("private_key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return "", err
	}
	key, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return "", errors.New("private_key is not an ECDSA key")
	}
	enc := base64.RawURLEncoding.EncodeToString
	hdr, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID, "typ": "JWT"})
	now := time.Now().Unix()
	claims, _ := json.Marshal(map[string]any{"iss": issuer, "iat": now, "exp": now + 600, "aud": "appstoreconnect-v1"})
	signing := enc(hdr) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64) // JWT wants raw r||s, 32 bytes each
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + enc(sig), nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	jwt, err := token(core.Str(cfg, "issuer_id"), core.Str(cfg, "key_id"), core.Str(cfg, "private_key"))
	if err != nil {
		return nil, err
	}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		var resp struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Version string `json:"versionString"`
					State   string `json:"appStoreState"`
					Created string `json:"createdDate"`
				} `json:"attributes"`
			} `json:"data"`
		}
		u := fmt.Sprintf("%s/v1/apps/%s/appStoreVersions?limit=5", baseURL, url.PathEscape(core.Str(t, "app_id")))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, map[string]string{"Authorization": "Bearer " + jwt}, &resp); err != nil {
			return out, err
		}
		for _, d := range resp.Data {
			ts, _ := time.Parse(time.RFC3339, d.Attributes.Created)
			stage, prog := stateProgress(d.Attributes.State)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: d.ID,
				Platform:   "appstore",
				Status:     mapState(d.Attributes.State),
				Version:    d.Attributes.Version,
				Stage:      stage,
				Progress:   prog,
				Message:    d.Attributes.State,
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	appID := core.Str(target, "app_id")
	cli := fmt.Sprintf(
		`curl -X POST "https://api.appstoreconnect.apple.com/v1/webhooks" -H "Authorization: Bearer $ASC_JWT" -H "Content-Type: application/json" -d '{"data":{"type":"webhooks","attributes":{"enabled":true,"eventTypes":["APP_STORE_VERSION_APP_VERSION_STATE_UPDATED","BUILD_UPLOAD_STATE_UPDATED","BUILD_BETA_DETAIL_EXTERNAL_BUILD_STATE_UPDATED"],"name":"VersionRadar","secret":"%s","url":"%s"},"relationships":{"app":{"data":{"id":"%s","type":"apps"}}}}}'`,
		secret, webhookURL, appID,
	)
	return core.WebhookStatus{
		Plugin:      "appstore",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("App Store Connect > Users and Access > Integrations > Webhooks > Add Webhook for App ID %s. URL: %s, Secret: %s, Events: App Store Version State Updated + Build Upload State Updated.", appID, webhookURL, secret),
	}
}

// ConnectWebhook registers a webhook via the App Store Connect API (requires Admin/Account Holder API key).
func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	appID := strings.TrimSpace(core.Str(target, "app_id"))
	if appID == "" {
		return "", errors.New("app_id is required")
	}
	jwt, err := token(core.Str(cfg, "issuer_id"), core.Str(cfg, "key_id"), core.Str(cfg, "private_key"))
	if err != nil {
		return "", err
	}
	hdr := map[string]string{"Authorization": "Bearer " + jwt}
	payload := map[string]any{
		"data": map[string]any{
			"type": "webhooks",
			"attributes": map[string]any{
				"enabled": true,
				"eventTypes": []string{
					"APP_STORE_VERSION_APP_VERSION_STATE_UPDATED",
					"BUILD_UPLOAD_STATE_UPDATED",
					"BUILD_BETA_DETAIL_EXTERNAL_BUILD_STATE_UPDATED",
				},
				"name":   "VersionRadar",
				"secret": secret,
				"url":    webhookURL,
			},
			"relationships": map[string]any{
				"app": map[string]any{
					"data": map[string]string{"id": appID, "type": "apps"},
				},
			},
		},
	}
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, baseURL+"/v1/webhooks", hdr, payload, &created); err != nil {
		return "", fmt.Errorf("create App Store Connect webhook (API key needs Admin role, or configure manually): %w", err)
	}
	return created.Data.ID, nil
}
