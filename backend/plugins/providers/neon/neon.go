// Package neon tracks Neon Serverless Postgres branches, compute operations, and optional CI migration pushes
// via the Neon API v2.
package neon

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

func (Plugin) Name() string    { return "neon" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:    "api_key",
			Label:  "Neon API Key",
			Help:   "Neon Console > Account Settings > API keys",
			Secret: true,
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project_id", Label: "Neon Project ID", Help: "e.g. misty-lake-12345678"},
		{Key: "branch_name", Label: "Branch name (optional)", Help: "e.g. main or production (leave empty to track all project operations)", Default: "main", Optional: true},
	}
}

var baseURL = "https://console.neon.tech/api/v2"

func headers(key string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(key),
		"Accept":        "application/json",
	}
}

// VerifyWebhook accepts optional CI migration notifications (e.g. after prisma/drizzle/goose migrate).
func (Plugin) VerifyWebhook(r *http.Request, _ []byte, secret string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-VersionRadar-Secret")), []byte(secret)) != 1 {
		return errors.New("bad X-VersionRadar-Secret")
	}
	return nil
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		ID        string `json:"id"`
		Version   string `json:"version"`
		CommitSHA string `json:"commit_sha"`
		Status    string `json:"status"`
		Branch    string `json:"branch"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	st := core.StatusSuccess
	stage := "migrated"
	prog := 100
	switch strings.ToLower(p.Status) {
	case "failed", "error":
		st = core.StatusFailed
		stage = "failed"
	case "in_progress", "running", "started":
		st = core.StatusInProgress
		stage = "migrating"
		prog = 50
	}
	extID := p.ID
	if extID == "" {
		extID = "mig:" + p.Branch + ":" + p.Version
	}
	msg := strings.TrimSpace(p.Message)
	if msg == "" {
		msg = strings.TrimSpace("Neon migration " + p.Branch)
	}
	return &core.DeploymentEvent{
		ExternalID: extID,
		Platform:   "neon",
		Status:     st,
		Version:    p.Version,
		CommitSHA:  p.CommitSHA,
		Stage:      stage,
		Progress:   prog,
		Message:    msg,
	}, nil
}

func mapOpStatus(s string) (status, stage string, progress int) {
	switch strings.ToLower(s) {
	case "finished":
		return core.StatusSuccess, "ready", 100
	case "failed", "error", "cancelled", "skipped":
		return core.StatusFailed, strings.ToLower(s), 100
	case "scheduling":
		return core.StatusInProgress, "scheduling", 25
	default:
		return core.StatusInProgress, "running", 60
	}
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := headers(core.Str(cfg, "api_key"))
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		proj := strings.TrimSpace(core.Str(t, "project_id"))
		if proj == "" {
			continue
		}
		branchFilter := strings.TrimSpace(core.Str(t, "branch_name"))
		pid := core.Str(t, "product")

		// 1. Fetch branches to resolve branch ID & live state.
		var brResp struct {
			Branches []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				CurrentState string `json:"current_state"`
				UpdatedAt    string `json:"updated_at"`
			} `json:"branches"`
		}
		brURL := fmt.Sprintf("%s/projects/%s/branches", baseURL, url.PathEscape(proj))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, brURL, hdr, &brResp); err != nil {
			return out, err
		}
		branchNameByID := map[string]string{}
		targetBranchID := ""
		for _, b := range brResp.Branches {
			branchNameByID[b.ID] = b.Name
			if branchFilter != "" && strings.EqualFold(b.Name, branchFilter) {
				targetBranchID = b.ID
			}
		}

		// 2. Fetch recent operations for the project (filtered to target branch if set).
		var opResp struct {
			Operations []struct {
				ID        string `json:"id"`
				BranchID  string `json:"branch_id"`
				Action    string `json:"action"`
				Status    string `json:"status"`
				Error     string `json:"error"`
				UpdatedAt string `json:"updated_at"`
			} `json:"operations"`
		}
		opURL := fmt.Sprintf("%s/projects/%s/operations?limit=10", baseURL, url.PathEscape(proj))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, opURL, hdr, &opResp); err != nil {
			return out, err
		}
		for _, op := range opResp.Operations {
			if targetBranchID != "" && op.BranchID != "" && op.BranchID != targetBranchID {
				continue
			}
			st, stage, prog := mapOpStatus(op.Status)
			bName := branchNameByID[op.BranchID]
			if bName == "" {
				bName = branchFilter
			}
			msg := strings.TrimSpace(op.Action + " " + bName)
			if op.Error != "" {
				msg = op.Error
			}
			ts, _ := time.Parse(time.RFC3339, op.UpdatedAt)
			ver := bName
			if ver == "" {
				ver = op.Action
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  pid,
				ExternalID: "op:" + op.ID,
				Platform:   "neon",
				Status:     st,
				Version:    ver,
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
	branch := core.Str(target, "branch_name")
	if branch == "" {
		branch = "main"
	}
	cli := fmt.Sprintf(
		`curl -X POST '%s' -H 'X-VersionRadar-Secret: %s' -H 'Content-Type: application/json' -d '{"version":"20261126_add_index","branch":"%s","status":"success","message":"DB migration applied"}'`,
		webhookURL, secret, branch,
	)
	return core.WebhookStatus{
		Plugin:      "neon",
		Kind:        "provider",
		Supported:   true,
		Automatable: false,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		Note:        "Neon branches & operations are polled automatically. Optionally call this webhook from your DB migration step (Prisma / Drizzle / Goose) to report exact schema versions.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("neon branch operations are polled automatically; copy the optional migration curl from Manual / CLI")
}
