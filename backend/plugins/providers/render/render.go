// Package render tracks Render.com service builds and deploys via REST API polling and webhooks.
package render

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func (Plugin) Name() string    { return "render" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "api_token", Label: "Render API Key", Help: "Render Dashboard > Account Settings > API Keys", Secret: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "service_id", Label: "Service ID", Help: "e.g. srv-cxxxxxxxxxxx (from the Render service URL)"},
	}
}

var baseURL = "https://api.render.com/v1"

func headers(tok string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(tok),
		"Accept":        "application/json",
	}
}

// VerifyWebhook verifies Render's webhook-id / webhook-timestamp / webhook-signature (Standard Webhooks / Svix HMAC-SHA256).
func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("missing secret")
	}
	id := r.Header.Get("webhook-id")
	ts := r.Header.Get("webhook-timestamp")
	sigs := r.Header.Get("webhook-signature")
	if id == "" || ts == "" || sigs == "" {
		return errors.New("missing webhook signature headers")
	}
	keyBytes := []byte(secret)
	if strings.HasPrefix(secret, "whsec_") {
		if dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_")); err == nil {
			keyBytes = dec
		}
	}
	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(id + "." + ts + "." + string(body)))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, part := range strings.Fields(sigs) {
		if _, val, ok := strings.Cut(part, ","); ok && hmac.Equal([]byte(val), []byte(want)) {
			return nil
		}
	}
	return errors.New("bad webhook-signature")
}

func mapDeployStatus(s string) (status, stage string, progress int) {
	switch strings.ToLower(s) {
	case "live", "succeeded":
		return core.StatusSuccess, "live", 100
	case "build_failed", "update_failed", "canceled", "deactivated", "pre_deploy_failed", "failed":
		return core.StatusFailed, strings.ToLower(s), 100
	case "created":
		return core.StatusInProgress, "queued", 15
	case "build_in_progress":
		return core.StatusInProgress, "building", 45
	case "pre_deploy_in_progress":
		return core.StatusInProgress, "pre_deploy", 70
	case "update_in_progress":
		return core.StatusInProgress, "deploying", 85
	default:
		return core.StatusInProgress, strings.ToLower(s), 50
	}
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Data      struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Commit struct {
				ID      string `json:"id"`
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	stStr := p.Data.Status
	if stStr == "" {
		switch p.Type {
		case "deploy_started", "build_started":
			stStr = "build_in_progress"
		case "deploy_ended":
			stStr = "live"
		}
	}
	status, stage, prog := mapDeployStatus(stStr)
	ts, _ := time.Parse(time.RFC3339, p.Timestamp)
	return &core.DeploymentEvent{
		ExternalID: p.Data.ID,
		Platform:   "render",
		Status:     status,
		Version:    core.ShortSHA(p.Data.Commit.ID, p.Data.ID),
		CommitSHA:  p.Data.Commit.ID,
		Stage:      stage,
		Progress:   prog,
		Message:    strings.Split(strings.TrimSpace(p.Data.Commit.Message), "\n")[0],
		Timestamp:  ts,
	}, nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := headers(core.Str(cfg, "api_token"))
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		svcID := strings.TrimSpace(core.Str(t, "service_id"))
		if svcID == "" {
			continue
		}
		var rows []struct {
			Deploy struct {
				ID        string `json:"id"`
				Status    string `json:"status"`
				CreatedAt string `json:"createdAt"`
				Commit    struct {
					ID      string `json:"id"`
					Message string `json:"message"`
				} `json:"commit"`
			} `json:"deploy"`
		}
		u := fmt.Sprintf("%s/services/%s/deploys?limit=10", baseURL, url.PathEscape(svcID))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &rows); err != nil {
			return out, err
		}
		for _, r := range rows {
			d := r.Deploy
			status, stage, prog := mapDeployStatus(d.Status)
			ts, _ := time.Parse(time.RFC3339, d.CreatedAt)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: d.ID,
				Platform:   "render",
				Status:     status,
				Version:    core.ShortSHA(d.Commit.ID, d.ID),
				CommitSHA:  d.Commit.ID,
				Stage:      stage,
				Progress:   prog,
				Message:    strings.Split(strings.TrimSpace(d.Commit.Message), "\n")[0],
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, _ string, target map[string]any) core.WebhookStatus {
	svc := core.Str(target, "service_id")
	cli := fmt.Sprintf(
		`curl -X POST "https://api.render.com/v1/webhooks" -H "Authorization: Bearer $RENDER_API_KEY" -H "Content-Type: application/json" -d '{"name":"VersionRadar %s","url":"%s","enabled":true,"eventFilter":["build_started","build_ended","deploy_started","deploy_ended"]}'`,
		svc, webhookURL,
	)
	return core.WebhookStatus{
		Plugin:      "render",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("Render Dashboard > Integrations > Webhooks > Create Webhook. URL: %s. Events: build_started, build_ended, deploy_started, deploy_ended.", webhookURL),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, _ string) (string, error) {
	svc := core.Str(target, "service_id")
	var created struct {
		ID string `json:"id"`
	}
	payload := map[string]any{
		"name":        "VersionRadar " + svc,
		"url":         webhookURL,
		"enabled":     true,
		"eventFilter": []string{"build_started", "build_ended", "deploy_started", "deploy_ended"},
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, baseURL+"/webhooks", headers(core.Str(cfg, "api_token")), payload, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}
