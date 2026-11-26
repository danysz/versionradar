// Package cloudflare handles Cloudflare Pages deployment polling, notifications, and webhook automation.
package cloudflare

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "cloudflare" }
func (Plugin) Version() string { return "1.1.0" }

// Cloudflare notification webhooks echo the destination's secret in the cf-webhook-auth header.
func (Plugin) VerifyWebhook(r *http.Request, _ []byte, secret string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("cf-webhook-auth")), []byte(secret)) != 1 {
		return errors.New("bad cf-webhook-auth")
	}
	return nil
}

// Payload shape (data.event "EVENT_DEPLOYMENT_STARTED", data.deployment_id, data.commit_hash, data.environment)
// follows Cloudflare Pages notification webhooks; succeeded/failed/stage names are matched loosely.
func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Text string `json:"text"`
		TS   int64  `json:"ts"`
		Data struct {
			Event        string `json:"event"`
			DeploymentID string `json:"deployment_id"`
			ProjectName  string `json:"project_name"`
			Environment  string `json:"environment"`
			CommitHash   string `json:"commit_hash"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	ev := strings.ToUpper(p.Data.Event)
	status := core.StatusInProgress
	stage := strings.ToLower(strings.TrimPrefix(p.Data.Event, "EVENT_DEPLOYMENT_"))
	progress := 25
	switch {
	case strings.Contains(ev, "FAIL") || strings.Contains(ev, "CANCEL"):
		status = core.StatusFailed
		progress = 100
	case strings.Contains(ev, "SUCCE"):
		status = core.StatusSuccess
		stage = "deployed"
		progress = 100
	case strings.Contains(ev, "DEPLOY"):
		progress = 80
	case strings.Contains(ev, "BUILD"):
		progress = 55
	case strings.Contains(ev, "START"):
		stage = "started"
		progress = 20
	}
	out := &core.DeploymentEvent{
		Platform:   "cloudflare",
		Status:     status,
		ExternalID: p.Data.DeploymentID,
		Version:    shortHash(p.Data.CommitHash, p.Data.DeploymentID),
		CommitSHA:  p.Data.CommitHash,
		Stage:      stage,
		Progress:   progress,
		Message:    strings.TrimSpace(p.Data.ProjectName + " " + strings.ToLower(strings.TrimPrefix(p.Data.Environment, "ENVIRONMENT_")) + " " + strings.ToLower(strings.TrimPrefix(p.Data.Event, "EVENT_DEPLOYMENT_"))),
	}
	if out.Message == "" {
		out.Message = p.Text
	}
	if p.TS > 0 {
		out.Timestamp = time.Unix(p.TS, 0)
	}
	return out, nil
}

func shortHash(hash, fallback string) string {
	return core.ShortSHA(hash, fallback)
}

func stageProgress(name, stageStatus string) int {
	if stageStatus == "failure" || stageStatus == "canceled" {
		return 100
	}
	switch name {
	case "queued":
		return 10
	case "initialize":
		return 25
	case "clone_repo":
		return 40
	case "build":
		if stageStatus == "success" {
			return 75
		}
		return 60
	case "deploy":
		if stageStatus == "success" {
			return 100
		}
		return 90
	default:
		return 50
	}
}

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "api_token", Label: "API token", Help: "Account > Cloudflare Pages: Read (add Notifications: Edit for 1-click webhook connect)", Secret: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "account_id", Label: "Account ID"},
		{Key: "project", Label: "Pages project name"},
	}
}

var baseURL = "https://api.cloudflare.com/client/v4"

// CheckStatus polls the Cloudflare Pages deployments API, mapping latest_stage to status & progress.
func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := map[string]string{"Authorization": "Bearer " + core.Str(cfg, "api_token")}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		var resp struct {
			Result []struct {
				ID          string `json:"id"`
				CreatedOn   string `json:"created_on"`
				Environment string `json:"environment"`
				Stage       struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				} `json:"latest_stage"`
				Trigger struct {
					Metadata struct {
						Hash    string `json:"commit_hash"`
						Message string `json:"commit_message"`
						Branch  string `json:"branch"`
					} `json:"metadata"`
				} `json:"deployment_trigger"`
			} `json:"result"`
		}
		u := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments?per_page=10", baseURL,
			url.PathEscape(core.Str(t, "account_id")), url.PathEscape(core.Str(t, "project")))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &resp); err != nil {
			return out, err
		}
		for _, d := range resp.Result {
			status := core.StatusInProgress
			switch {
			case d.Stage.Status == "skipped":
				continue
			case d.Stage.Status == "failure" || d.Stage.Status == "canceled":
				status = core.StatusFailed
			case d.Stage.Status == "success" && d.Stage.Name == "deploy":
				status = core.StatusSuccess
			}
			msg := d.Environment + " " + d.Trigger.Metadata.Branch
			if c, _, _ := strings.Cut(d.Trigger.Metadata.Message, "\n"); c != "" {
				msg += ": " + c
			}
			ts, _ := time.Parse(time.RFC3339, d.CreatedOn)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: d.ID,
				Platform:   "cloudflare",
				Status:     status,
				Version:    shortHash(d.Trigger.Metadata.Hash, d.ID),
				CommitSHA:  d.Trigger.Metadata.Hash,
				Stage:      d.Stage.Name,
				Progress:   stageProgress(d.Stage.Name, d.Stage.Status),
				Message:    strings.TrimSpace(msg),
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	acct := core.Str(target, "account_id")
	proj := core.Str(target, "project")
	cli := fmt.Sprintf(
		`DEST_ID=$(curl -s -X POST "https://api.cloudflare.com/client/v4/accounts/%s/alerting/v3/destinations/webhooks" -H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json" -d '{"name":"VersionRadar %s","url":"%s","secret":"%s"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["result"]["id"])') && curl -s -X POST "https://api.cloudflare.com/client/v4/accounts/%s/alerting/v3/policies" -H "Authorization: Bearer $CF_API_TOKEN" -H "Content-Type: application/json" -d "{\"name\":\"VersionRadar %s\",\"alert_type\":\"pages_event_alert\",\"enabled\":true,\"mechanisms\":{\"webhooks\":[{\"id\":\"$DEST_ID\"}]},\"filters\":{\"event\":[\"EVENT_DEPLOYMENT_STARTED\",\"EVENT_DEPLOYMENT_FAILED\",\"EVENT_DEPLOYMENT_SUCCESS\"]}}"`,
		acct, proj, webhookURL, secret, acct, proj,
	)
	return core.WebhookStatus{
		Plugin:      "cloudflare",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("Cloudflare Dashboard > Notifications > Destinations > Webhooks > Create (URL: %s, Secret: %s). Then Add Notification > Pages > select project %s and all deployment events (Started, Failed, Succeeded).", webhookURL, secret, proj),
	}
}

// ConnectWebhook creates a Cloudflare Notifications webhook destination + pages_event_alert policy.
func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	acct := strings.TrimSpace(core.Str(target, "account_id"))
	proj := strings.TrimSpace(core.Str(target, "project"))
	if acct == "" || proj == "" {
		return "", errors.New("account_id and project are required")
	}
	hdr := map[string]string{"Authorization": "Bearer " + core.Str(cfg, "api_token")}

	// 1. Find or create webhook destination.
	destURL := fmt.Sprintf("%s/accounts/%s/alerting/v3/destinations/webhooks", baseURL, url.PathEscape(acct))
	var existingDests struct {
		Result []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"result"`
	}
	destID := ""
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, destURL, hdr, &existingDests); err == nil {
		for _, d := range existingDests.Result {
			if d.URL == webhookURL {
				destID = d.ID
				break
			}
		}
	}
	if destID == "" {
		var createdDest struct {
			Result struct {
				ID string `json:"id"`
			} `json:"result"`
		}
		err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, destURL, hdr, map[string]any{
			"name":   "VersionRadar " + proj,
			"url":    webhookURL,
			"secret": secret,
		}, &createdDest)
		if err != nil {
			return "", fmt.Errorf("create cloudflare webhook destination (needs Notifications:Edit): %w", err)
		}
		destID = createdDest.Result.ID
	}

	// 2. Create pages_event_alert policy for started, failed, and succeeded events.
	polURL := fmt.Sprintf("%s/accounts/%s/alerting/v3/policies", baseURL, url.PathEscape(acct))
	var createdPol struct {
		Result struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, polURL, hdr, map[string]any{
		"name":       "VersionRadar " + proj,
		"alert_type": "pages_event_alert",
		"enabled":    true,
		"mechanisms": map[string]any{
			"webhooks": []map[string]string{{"id": destID}},
		},
		"filters": map[string]any{
			"event": []string{
				"EVENT_DEPLOYMENT_STARTED",
				"EVENT_DEPLOYMENT_FAILED",
				"EVENT_DEPLOYMENT_SUCCESS",
			},
		},
	}, &createdPol)
	if err != nil {
		return "", fmt.Errorf("create cloudflare notification policy: %w", err)
	}
	return createdPol.Result.ID, nil
}
