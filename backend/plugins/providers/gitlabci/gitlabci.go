// Package gitlabci tracks GitLab CI/CD pipelines via REST API polling and pipeline webhooks.
package gitlabci

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

type Plugin struct{}

func (Plugin) Name() string    { return "gitlab_ci" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "token", Label: "GitLab Access Token", Help: "read_api scope (use api scope for 1-click webhook connect)", Secret: true},
		{Key: "base_url", Label: "GitLab Base URL", Help: "Leave empty for https://gitlab.com", Optional: true, Default: "https://gitlab.com"},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project", Label: "Project path or ID", Help: "e.g. group/subgroup/app"},
		{Key: "ref", Label: "Branch/ref filter (optional)", Help: "e.g. main", Optional: true},
	}
}

var defaultBaseURL = "https://gitlab.com"

func apiBase(cfg map[string]any) string {
	b := strings.TrimSpace(core.Str(cfg, "base_url"))
	if b == "" {
		b = defaultBaseURL
	}
	return strings.TrimRight(b, "/") + "/api/v4"
}

func headers(tok string) map[string]string {
	return map[string]string{"PRIVATE-TOKEN": strings.TrimSpace(tok)}
}

func (Plugin) VerifyWebhook(r *http.Request, _ []byte, secret string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gitlab-Token")), []byte(secret)) != 1 {
		return errors.New("bad X-Gitlab-Token")
	}
	return nil
}

func mapPipelineStatus(s string, doneJobs, totalJobs int) (string, string, int) {
	switch strings.ToLower(s) {
	case "success":
		return core.StatusSuccess, "completed", 100
	case "failed", "canceled", "cancelled":
		return core.StatusFailed, strings.ToLower(s), 100
	case "created", "waiting_for_resource", "preparing", "pending":
		return core.StatusInProgress, "queued", 15
	default:
		if totalJobs > 0 {
			pct := 20 + (doneJobs*70)/totalJobs
			return fmt.Sprintf("running (%d/%d)", doneJobs, totalJobs), pctString(pct), pct
		}
		return core.StatusInProgress, "running", 55
	}
}

func pctString(int) string { return "running" }

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		ObjectAttributes struct {
			ID        int64  `json:"id"`
			Ref       string `json:"ref"`
			SHA       string `json:"sha"`
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
		} `json:"object_attributes"`
		Builds []struct {
			Status string `json:"status"`
		} `json:"builds"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	done := 0
	for _, b := range p.Builds {
		if b.Status == "success" {
			done++
		}
	}
	st, stage, prog := mapPipelineStatus(p.ObjectAttributes.Status, done, len(p.Builds))
	if st != core.StatusSuccess && st != core.StatusFailed {
		if len(p.Builds) > 0 {
			stage = fmt.Sprintf("jobs %d/%d", done, len(p.Builds))
		}
		st = core.StatusInProgress
	}
	msg := strings.Split(strings.TrimSpace(p.Commit.Message), "\n")[0]
	if msg == "" {
		msg = "Pipeline " + p.ObjectAttributes.Ref
	}
	return &core.DeploymentEvent{
		ExternalID: fmt.Sprintf("%d", p.ObjectAttributes.ID),
		Platform:   "gitlab_ci",
		Status:     st,
		Version:    core.ShortSHA(p.ObjectAttributes.SHA, fmt.Sprintf("#%d", p.ObjectAttributes.ID)),
		CommitSHA:  p.ObjectAttributes.SHA,
		Stage:      stage,
		Progress:   prog,
		Message:    msg,
	}, nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	base := apiBase(cfg)
	hdr := headers(core.Str(cfg, "token"))
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		proj := strings.TrimSpace(strings.TrimSuffix(core.Str(t, "project"), ".git"))
		if proj == "" {
			continue
		}
		u := fmt.Sprintf("%s/projects/%s/pipelines?per_page=10", base, url.PathEscape(proj))
		if ref := strings.TrimSpace(core.Str(t, "ref")); ref != "" {
			u += "&ref=" + url.QueryEscape(ref)
		}
		var pipes []struct {
			ID        int64  `json:"id"`
			Ref       string `json:"ref"`
			SHA       string `json:"sha"`
			Status    string `json:"status"`
			UpdatedAt string `json:"updated_at"`
		}
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &pipes); err != nil {
			return out, err
		}
		for _, pl := range pipes {
			st, stage, prog := mapPipelineStatus(pl.Status, 0, 0)
			ts, _ := time.Parse(time.RFC3339, pl.UpdatedAt)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: fmt.Sprintf("%d", pl.ID),
				Platform:   "gitlab_ci",
				Status:     st,
				Version:    core.ShortSHA(pl.SHA, fmt.Sprintf("#%d", pl.ID)),
				CommitSHA:  pl.SHA,
				Stage:      stage,
				Progress:   prog,
				Message:    fmt.Sprintf("Pipeline %s (%s)", pl.Ref, pl.Status),
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	proj := core.Str(target, "project")
	cli := fmt.Sprintf(
		`curl -X POST "https://gitlab.com/api/v4/projects/%s/hooks" -H "PRIVATE-TOKEN: $GITLAB_TOKEN" -H "Content-Type: application/json" -d '{"url":"%s","token":"%s","pipeline_events":true,"push_events":false}'`,
		url.PathEscape(proj), webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "gitlab_ci",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("GitLab project %s > Settings > Webhooks > Add new webhook. URL: %s, Secret token: %s, Trigger: Pipeline events.", proj, webhookURL, secret),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	proj := strings.TrimSpace(strings.TrimSuffix(core.Str(target, "project"), ".git"))
	if proj == "" {
		return "", errors.New("project is required")
	}
	u := fmt.Sprintf("%s/projects/%s/hooks", apiBase(cfg), url.PathEscape(proj))
	var created struct {
		ID int64 `json:"id"`
	}
	payload := map[string]any{
		"url":             webhookURL,
		"token":           secret,
		"pipeline_events": true,
		"push_events":     false,
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, u, headers(core.Str(cfg, "token")), payload, &created); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", created.ID), nil
}
