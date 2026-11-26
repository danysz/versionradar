// Package githubactions tracks GitHub Actions workflow runs via REST API polling and workflow_run webhooks.
package githubactions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

func (Plugin) Name() string    { return "github_actions" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "token", Label: "GitHub Token", Help: "Fine-grained PAT with Actions: Read (add Webhooks: Write for 1-click connect)", Secret: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "repo", Label: "Repository (owner/name)", Help: "e.g. octocat/api"},
		{Key: "workflow", Label: "Workflow file (optional)", Help: "e.g. deploy.yml (leave empty to watch all workflows)", Optional: true},
		{Key: "branch", Label: "Branch filter (optional)", Help: "e.g. main", Optional: true},
	}
}

var baseURL = "https://api.github.com"

func headers(token string) map[string]string {
	h := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
		"User-Agent":           "VersionRadar",
	}
	if strings.TrimSpace(token) != "" {
		h["Authorization"] = "Bearer " + strings.TrimSpace(token)
	}
	return h
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(strings.TrimSuffix(repo, ".git")), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("repo must be owner/name")
	}
	return parts[0], parts[1], nil
}

func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("missing secret")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(r.Header.Get("X-Hub-Signature-256")), []byte(want)) {
		return errors.New("bad X-Hub-Signature-256")
	}
	return nil
}

func mapRunStatus(status, conclusion string) (string, string, int) {
	switch strings.ToLower(status) {
	case "completed":
		switch strings.ToLower(conclusion) {
		case "success":
			return core.StatusSuccess, "completed", 100
		case "failure", "timed_out", "cancelled", "action_required":
			return core.StatusFailed, strings.ToLower(conclusion), 100
		default:
			return core.StatusSuccess, strings.ToLower(conclusion), 100
		}
	case "queued", "requested", "waiting", "pending":
		return core.StatusInProgress, "queued", 15
	default:
		return core.StatusInProgress, "in_progress", 55
	}
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Action      string `json:"action"`
		WorkflowRun *struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			HeadBranch string `json:"head_branch"`
			HeadSHA    string `json:"head_sha"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			UpdatedAt  string `json:"updated_at"`
			Display    string `json:"display_title"`
		} `json:"workflow_run"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.WorkflowRun == nil {
		return nil, errors.New("missing workflow_run in payload")
	}
	run := p.WorkflowRun
	st, stage, prog := mapRunStatus(run.Status, run.Conclusion)
	ts, _ := time.Parse(time.RFC3339, run.UpdatedAt)
	msg := strings.TrimSpace(run.Name)
	if run.Display != "" {
		msg = strings.TrimSpace(msg + ": " + run.Display)
	}
	return &core.DeploymentEvent{
		ExternalID: fmt.Sprintf("%d", run.ID),
		Platform:   "github_actions",
		Status:     st,
		Version:    core.ShortSHA(run.HeadSHA, fmt.Sprintf("#%d", run.ID)),
		CommitSHA:  run.HeadSHA,
		Stage:      stage,
		Progress:   prog,
		Message:    msg,
		Timestamp:  ts,
	}, nil
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := headers(core.Str(cfg, "token"))
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		owner, name, err := splitRepo(core.Str(t, "repo"))
		if err != nil {
			return out, err
		}
		wf := strings.TrimSpace(core.Str(t, "workflow"))
		branch := strings.TrimSpace(core.Str(t, "branch"))

		u := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=10", baseURL, url.PathEscape(owner), url.PathEscape(name))
		if wf != "" {
			u = fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=10", baseURL, url.PathEscape(owner), url.PathEscape(name), url.PathEscape(wf))
		}
		if branch != "" {
			u += "&branch=" + url.QueryEscape(branch)
		}
		var resp struct {
			Runs []struct {
				ID         int64  `json:"id"`
				Name       string `json:"name"`
				HeadBranch string `json:"head_branch"`
				HeadSHA    string `json:"head_sha"`
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
				UpdatedAt  string `json:"updated_at"`
				Display    string `json:"display_title"`
			} `json:"workflow_runs"`
		}
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &resp); err != nil {
			return out, err
		}
		for _, run := range resp.Runs {
			st, stage, prog := mapRunStatus(run.Status, run.Conclusion)
			ts, _ := time.Parse(time.RFC3339, run.UpdatedAt)
			msg := strings.TrimSpace(run.Name)
			if run.Display != "" {
				msg = strings.TrimSpace(msg + ": " + run.Display)
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: fmt.Sprintf("%d", run.ID),
				Platform:   "github_actions",
				Status:     st,
				Version:    core.ShortSHA(run.HeadSHA, fmt.Sprintf("#%d", run.ID)),
				CommitSHA:  run.HeadSHA,
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
	repo := core.Str(target, "repo")
	cli := fmt.Sprintf(
		`gh api repos/%s/hooks --method POST -f name=web -F active=true -f 'events[]=workflow_run' -f 'config[url]=%s' -f 'config[content_type]=json' -f 'config[secret]=%s'`,
		repo, webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "github_actions",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("GitHub repo %s > Settings > Webhooks > Add webhook. URL: %s, Content type: application/json, Secret: %s, Events: Workflow runs.", repo, webhookURL, secret),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	owner, name, err := splitRepo(core.Str(target, "repo"))
	if err != nil {
		return "", err
	}
	hdr := headers(core.Str(cfg, "token"))
	u := fmt.Sprintf("%s/repos/%s/%s/hooks", baseURL, url.PathEscape(owner), url.PathEscape(name))
	var created struct {
		ID int64 `json:"id"`
	}
	payload := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"workflow_run"},
		"config": map[string]any{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, u, hdr, payload, &created); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", created.ID), nil
}
