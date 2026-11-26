// Package netlify tracks Netlify site deploys via REST API polling and signed deploy webhooks.
package netlify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{}

func (Plugin) Name() string    { return "netlify" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "api_token", Label: "Personal Access Token", Help: "Netlify User Settings > Applications > Personal access tokens", Secret: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "site_id", Label: "Site ID", Help: "Site configuration > Site details > Site ID (UUID)"},
	}
}

var baseURL = "https://api.netlify.com/api/v1"

func authHeaders(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(tok)}
}

// VerifyWebhook validates Netlify's X-Webhook-Signature JWS (HS256 JWT containing sha256 of body).
func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("missing secret")
	}
	tok := strings.TrimSpace(r.Header.Get("X-Webhook-Signature"))
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return errors.New("bad X-Webhook-Signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(wantSig)) {
		return errors.New("invalid webhook JWT signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var claims struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if !hmac.Equal([]byte(claims.SHA256), []byte(hex.EncodeToString(sum[:]))) {
		return errors.New("body hash mismatch")
	}
	return nil
}

func mapState(state string) (status, stage string, progress int) {
	switch strings.ToLower(state) {
	case "ready":
		return core.StatusSuccess, "ready", 100
	case "error", "failed":
		return core.StatusFailed, "failed", 100
	case "enqueued", "new":
		return core.StatusInProgress, "queued", 15
	case "uploading", "uploaded":
		return core.StatusInProgress, "uploading", 80
	default:
		return core.StatusInProgress, "building", 50
	}
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var d struct {
		ID           string `json:"id"`
		State        string `json:"state"`
		Context      string `json:"context"`
		Branch       string `json:"branch"`
		CommitRef    string `json:"commit_ref"`
		Title        string `json:"title"`
		ErrorMessage string `json:"error_message"`
		UpdatedAt    string `json:"updated_at"`
		CreatedAt    string `json:"created_at"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, err
	}
	status, stage, prog := mapState(d.State)
	ts, _ := time.Parse(time.RFC3339, d.UpdatedAt)
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339, d.CreatedAt)
	}
	msg := d.ErrorMessage
	if msg == "" {
		msg = strings.TrimSpace(d.Context + " " + d.Branch)
		if d.Title != "" {
			msg = strings.TrimSpace(msg + ": " + d.Title)
		}
	}
	return &core.DeploymentEvent{
		ExternalID: d.ID,
		Platform:   "netlify",
		Status:     status,
		Version:    core.ShortSHA(d.CommitRef, d.ID),
		CommitSHA:  d.CommitRef,
		Stage:      stage,
		Progress:   prog,
		Message:    msg,
		Timestamp:  ts,
	}, nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := authHeaders(core.Str(cfg, "api_token"))
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		siteID := strings.TrimSpace(core.Str(t, "site_id"))
		if siteID == "" {
			continue
		}
		var deploys []struct {
			ID           string `json:"id"`
			State        string `json:"state"`
			Context      string `json:"context"`
			Branch       string `json:"branch"`
			CommitRef    string `json:"commit_ref"`
			Title        string `json:"title"`
			ErrorMessage string `json:"error_message"`
			CreatedAt    string `json:"created_at"`
		}
		u := fmt.Sprintf("%s/sites/%s/deploys?per_page=10", baseURL, url.PathEscape(siteID))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &deploys); err != nil {
			return out, err
		}
		for _, d := range deploys {
			status, stage, prog := mapState(d.State)
			ts, _ := time.Parse(time.RFC3339, d.CreatedAt)
			msg := d.ErrorMessage
			if msg == "" {
				msg = strings.TrimSpace(d.Context + " " + d.Branch)
				if d.Title != "" {
					msg = strings.TrimSpace(msg + ": " + d.Title)
				}
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: d.ID,
				Platform:   "netlify",
				Status:     status,
				Version:    core.ShortSHA(d.CommitRef, d.ID),
				CommitSHA:  d.CommitRef,
				Stage:      stage,
				Progress:   prog,
				Message:    msg,
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	siteID := core.Str(target, "site_id")
	cli := fmt.Sprintf(
		`for ev in deploy_building deploy_created deploy_failed; do curl -s -X POST "https://api.netlify.com/api/v1/hooks?site_id=%s" -H "Authorization: Bearer $NETLIFY_TOKEN" -H "Content-Type: application/json" -d "{\"type\":\"url\",\"event\":\"$ev\",\"data\":{\"url\":\"%s\",\"jws_secret\":\"%s\"}}"; done`,
		url.QueryEscape(siteID), webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "netlify",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("Netlify Site %s > Site configuration > Notifications > Deploy notifications > Add HTTP POST request for Deploy started, Deploy succeeded, Deploy failed. URL: %s, JWS Secret: %s.", siteID, webhookURL, secret),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	siteID := strings.TrimSpace(core.Str(target, "site_id"))
	if siteID == "" {
		return "", errors.New("site_id is required")
	}
	hdr := authHeaders(core.Str(cfg, "api_token"))
	u := fmt.Sprintf("%s/hooks?site_id=%s", baseURL, url.QueryEscape(siteID))
	var ids []string
	for _, ev := range []string{"deploy_building", "deploy_created", "deploy_failed"} {
		var created struct {
			ID string `json:"id"`
		}
		payload := map[string]any{
			"type":  "url",
			"event": ev,
			"data": map[string]any{
				"url":        webhookURL,
				"jws_secret": secret,
			},
		}
		if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, u, hdr, payload, &created); err != nil {
			return "", err
		}
		ids = append(ids, created.ID)
	}
	return strings.Join(ids, ","), nil
}
