// Package vercel tracks Vercel deployments via REST API polling and x-vercel-signature webhooks.
package vercel

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
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

func (Plugin) Name() string    { return "vercel" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "api_token", Label: "Vercel Access Token", Help: "Account Settings > Tokens", Secret: true},
		{Key: "team_id", Label: "Team ID (optional)", Help: "Required only for Vercel team projects", Optional: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project_id", Label: "Project ID or Name", Help: "e.g. prj_... or my-web-app"},
	}
}

var baseURL = "https://api.vercel.com"

func authHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(token)}
}

func teamQuery(cfg map[string]any) string {
	if tid := strings.TrimSpace(core.Str(cfg, "team_id")); tid != "" {
		return "&teamId=" + url.QueryEscape(tid)
	}
	return ""
}

// VerifyWebhook checks Vercel's x-vercel-signature header (HMAC-SHA1 hex of body using webhook secret).
func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("missing secret")
	}
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(r.Header.Get("x-vercel-signature"))), []byte(want)) {
		return errors.New("bad x-vercel-signature")
	}
	return nil
}

func mapEventStatus(evType string) (status, stage string, progress int) {
	switch strings.ToLower(evType) {
	case "deployment.succeeded", "deployment.ready", "ready":
		return core.StatusSuccess, "ready", 100
	case "deployment.error", "deployment.canceled", "error", "canceled":
		return core.StatusFailed, strings.TrimPrefix(strings.ToLower(evType), "deployment."), 100
	case "deployment.created", "queued", "initializing":
		return core.StatusInProgress, "queued", 20
	default:
		return core.StatusInProgress, "building", 60
	}
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Type      string `json:"type"`
		CreatedAt int64  `json:"createdAt"`
		Payload   struct {
			Target     string `json:"target"`
			Deployment struct {
				ID   string            `json:"id"`
				Name string            `json:"name"`
				URL  string            `json:"url"`
				Meta map[string]string `json:"meta"`
			} `json:"deployment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	status, stage, prog := mapEventStatus(p.Type)
	meta := p.Payload.Deployment.Meta
	sha := meta["githubCommitSha"]
	if sha == "" {
		sha = meta["gitlabCommitSha"]
	}
	msg := meta["githubCommitMessage"]
	if msg == "" {
		msg = meta["gitlabCommitMessage"]
	}
	if msg == "" {
		msg = strings.TrimSpace(p.Payload.Target + " " + p.Payload.Deployment.URL)
	}
	var ts time.Time
	if p.CreatedAt > 0 {
		ts = time.UnixMilli(p.CreatedAt)
	}
	return &core.DeploymentEvent{
		ExternalID: p.Payload.Deployment.ID,
		Platform:   "vercel",
		Status:     status,
		Version:    core.ShortSHA(sha, p.Payload.Deployment.ID),
		CommitSHA:  sha,
		Stage:      stage,
		Progress:   prog,
		Message:    strings.Split(msg, "\n")[0],
		Timestamp:  ts,
	}, nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := authHeaders(core.Str(cfg, "api_token"))
	tq := teamQuery(cfg)
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		proj := strings.TrimSpace(core.Str(t, "project_id"))
		if proj == "" {
			continue
		}
		var resp struct {
			Deployments []struct {
				UID     string            `json:"uid"`
				State   string            `json:"state"`
				Ready   string            `json:"readyState"`
				Target  string            `json:"target"`
				Created int64             `json:"created"`
				Meta    map[string]string `json:"meta"`
			} `json:"deployments"`
		}
		u := fmt.Sprintf("%s/v6/deployments?projectId=%s&limit=10%s", baseURL, url.QueryEscape(proj), tq)
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &resp); err != nil {
			return out, err
		}
		for _, d := range resp.Deployments {
			st := d.Ready
			if st == "" {
				st = d.State
			}
			status, stage, prog := mapEventStatus(st)
			sha := d.Meta["githubCommitSha"]
			if sha == "" {
				sha = d.Meta["gitlabCommitSha"]
			}
			msg := d.Meta["githubCommitMessage"]
			if msg == "" {
				msg = d.Meta["gitlabCommitMessage"]
			}
			if d.Target != "" && msg != "" {
				msg = d.Target + ": " + strings.Split(msg, "\n")[0]
			}
			var ts time.Time
			if d.Created > 0 {
				ts = time.UnixMilli(d.Created)
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: d.UID,
				Platform:   "vercel",
				Status:     status,
				Version:    core.ShortSHA(sha, d.UID),
				CommitSHA:  sha,
				Stage:      stage,
				Progress:   prog,
				Message:    strings.TrimSpace(msg),
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, _ string, target map[string]any) core.WebhookStatus {
	proj := core.Str(target, "project_id")
	cli := fmt.Sprintf(
		`curl -X POST "https://api.vercel.com/v1/webhooks" -H "Authorization: Bearer $VERCEL_TOKEN" -H "Content-Type: application/json" -d '{"url":"%s","events":["deployment.created","deployment.succeeded","deployment.error","deployment.canceled"],"projectIds":["%s"]}'`,
		webhookURL, proj,
	)
	return core.WebhookStatus{
		Plugin:      "vercel",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("Vercel Dashboard > Settings > Webhooks > Create Webhook for project %s. URL: %s. Events: deployment.created, deployment.succeeded, deployment.error, deployment.canceled.", proj, webhookURL),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, _ string) (string, error) {
	proj := strings.TrimSpace(core.Str(target, "project_id"))
	if proj == "" {
		return "", errors.New("project_id is required")
	}
	hdr := authHeaders(core.Str(cfg, "api_token"))
	u := baseURL + "/v1/webhooks"
	if tid := strings.TrimSpace(core.Str(cfg, "team_id")); tid != "" {
		u += "?teamId=" + url.QueryEscape(tid)
	}
	var created struct {
		ID string `json:"id"`
	}
	payload := map[string]any{
		"url":        webhookURL,
		"events":     []string{"deployment.created", "deployment.succeeded", "deployment.error", "deployment.canceled"},
		"projectIds": []string{proj},
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, u, hdr, payload, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}
