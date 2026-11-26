// Package heroku tracks Heroku builds and releases via the Heroku Platform API and App Webhooks.
package heroku

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

func (Plugin) Name() string    { return "heroku" }
func (Plugin) Version() string { return "1.0.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:    "api_token",
			Label:  "Heroku API Key / OAuth Token",
			Help:   "From Heroku Account Settings > API Key (or `heroku authorizations:create`)",
			Secret: true,
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "app_name", Label: "Heroku App Name", Help: "App name or UUID, e.g. my-api-prod"},
	}
}

var baseURL = "https://api.heroku.com"

func headers(token string, extra map[string]string) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(token),
		"Accept":        "application/vnd.heroku+json; version=3",
	}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// VerifyWebhook validates Heroku's Heroku-Webhook-Hmac-SHA256 header (base64 HMAC-SHA256 of raw body).
func (Plugin) VerifyWebhook(r *http.Request, body []byte, secret string) error {
	if secret == "" {
		return errors.New("missing webhook secret")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	got := strings.TrimSpace(r.Header.Get("Heroku-Webhook-Hmac-SHA256"))
	if !hmac.Equal([]byte(got), []byte(want)) {
		return errors.New("bad Heroku-Webhook-Hmac-SHA256")
	}
	return nil
}

// ParseWebhook handles Heroku `api:build` (start/finish) and `api:release` (deploy/finish) notifications.
func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var p struct {
		Action    string `json:"action"`
		Resource  string `json:"resource"`
		CreatedAt string `json:"created_at"`
		Data      struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			Version     int    `json:"version"`
			Description string `json:"description"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
			SourceBlob  struct {
				Version string `json:"version"`
			} `json:"source_blob"`
			Release *struct {
				ID string `json:"id"`
			} `json:"release"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	ts, _ := time.Parse(time.RFC3339, p.Data.UpdatedAt)
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339, p.Data.CreatedAt)
	}
	if ts.IsZero() {
		ts, _ = time.Parse(time.RFC3339, p.CreatedAt)
	}

	switch p.Resource {
	case "build":
		status, stage, prog := mapBuildStatus(p.Data.Status)
		ver := core.ShortSHA(p.Data.SourceBlob.Version, p.Data.ID)
		if p.Data.SourceBlob.Version != "" && !looksHex(p.Data.SourceBlob.Version) {
			ver = p.Data.SourceBlob.Version
		}
		commitSHA := ""
		if looksHex(p.Data.SourceBlob.Version) {
			commitSHA = p.Data.SourceBlob.Version
		}
		extID := "build:" + p.Data.ID
		if p.Data.Release != nil && p.Data.Release.ID != "" {
			extID = "release:" + p.Data.Release.ID
		}
		return &core.DeploymentEvent{
			ExternalID: extID,
			Platform:   "heroku",
			Status:     status,
			Version:    ver,
			CommitSHA:  commitSHA,
			Stage:      stage,
			Progress:   prog,
			Message:    strings.TrimSpace("Heroku build " + p.Data.Status),
			Timestamp:  ts,
		}, nil

	case "release":
		status, stage, prog := mapReleaseStatus(p.Data.Status)
		ver := fmt.Sprintf("v%d", p.Data.Version)
		if p.Data.Version == 0 {
			ver = p.Data.ID
		}
		commitSHA := extractDeployCommit(p.Data.Description)
		msg := strings.TrimSpace(p.Data.Description)
		if msg == "" {
			msg = fmt.Sprintf("Heroku release %s (%s)", ver, p.Data.Status)
		}
		return &core.DeploymentEvent{
			ExternalID: "release:" + p.Data.ID,
			Platform:   "heroku",
			Status:     status,
			Version:    ver,
			CommitSHA:  commitSHA,
			Stage:      stage,
			Progress:   prog,
			Message:    msg,
			Timestamp:  ts,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported heroku webhook resource %q", p.Resource)
	}
}

func mapBuildStatus(s string) (status, stage string, progress int) {
	switch strings.ToLower(s) {
	case "succeeded":
		return core.StatusInProgress, "built", 75
	case "failed":
		return core.StatusFailed, "build_failed", 100
	default:
		return core.StatusInProgress, "building", 35
	}
}

func mapReleaseStatus(s string) (status, stage string, progress int) {
	switch strings.ToLower(s) {
	case "succeeded":
		return core.StatusSuccess, "released", 100
	case "failed":
		return core.StatusFailed, "release_failed", 100
	default:
		return core.StatusInProgress, "releasing", 85
	}
}

func looksHex(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// extractDeployCommit pulls the short/full SHA out of typical Heroku descriptions like "Deploy a1b2c3d4".
func extractDeployCommit(desc string) string {
	for _, tok := range strings.Fields(desc) {
		tok = strings.Trim(tok, "(),.:;")
		if looksHex(tok) {
			return tok
		}
	}
	return ""
}

// CheckStatus polls both Heroku releases and builds to capture published versions, commit SHAs, and active/failed builds.
func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	tok := core.Str(cfg, "api_token")
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		app := strings.TrimSpace(core.Str(t, "app_name"))
		if app == "" {
			continue
		}
		var builds []struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
			SourceBlob struct {
				Version string `json:"version"`
			} `json:"source_blob"`
			Release *struct {
				ID string `json:"id"`
			} `json:"release"`
		}
		buildsURL := fmt.Sprintf("%s/apps/%s/builds", baseURL, url.PathEscape(app))
		_ = core.Do(ctx, core.DefaultClient, http.MethodGet, buildsURL,
			headers(tok, map[string]string{"Range": "created_at ..; order=desc, max=10"}), &builds)

		buildVersionByRelease := map[string]string{}
		for _, b := range builds {
			if b.Release != nil && b.Release.ID != "" && b.SourceBlob.Version != "" {
				buildVersionByRelease[b.Release.ID] = b.SourceBlob.Version
				continue
			}
			// Emit builds that don't have a release yet (in progress or failed builds).
			if b.Status == "pending" || b.Status == "failed" {
				status, stage, prog := mapBuildStatus(b.Status)
				ts, _ := time.Parse(time.RFC3339, b.UpdatedAt)
				if ts.IsZero() {
					ts, _ = time.Parse(time.RFC3339, b.CreatedAt)
				}
				ver := core.ShortSHA(b.SourceBlob.Version, b.ID)
				if b.SourceBlob.Version != "" && !looksHex(b.SourceBlob.Version) {
					ver = b.SourceBlob.Version
				}
				commitSHA := ""
				if looksHex(b.SourceBlob.Version) {
					commitSHA = b.SourceBlob.Version
				}
				out = append(out, core.DeploymentEvent{
					ProductID:  core.Str(t, "product"),
					ExternalID: "build:" + b.ID,
					Platform:   "heroku",
					Status:     status,
					Version:    ver,
					CommitSHA:  commitSHA,
					Stage:      stage,
					Progress:   prog,
					Message:    "Heroku build " + b.Status,
					Timestamp:  ts,
				})
			}
		}

		var releases []struct {
			ID          string `json:"id"`
			Version     int    `json:"version"`
			Status      string `json:"status"`
			Description string `json:"description"`
			CreatedAt   string `json:"created_at"`
		}
		relURL := fmt.Sprintf("%s/apps/%s/releases", baseURL, url.PathEscape(app))
		if err := core.Do(ctx, core.DefaultClient, http.MethodGet, relURL,
			headers(tok, map[string]string{"Range": "version ..; order=desc, max=10"}), &releases); err != nil {
			return out, err
		}
		for _, r := range releases {
			status, stage, prog := mapReleaseStatus(r.Status)
			ts, _ := time.Parse(time.RFC3339, r.CreatedAt)
			ver := fmt.Sprintf("v%d", r.Version)
			srcVer := buildVersionByRelease[r.ID]
			commitSHA := ""
			if looksHex(srcVer) {
				commitSHA = srcVer
			} else if srcVer != "" {
				ver = srcVer + " (" + ver + ")"
			}
			if commitSHA == "" {
				commitSHA = extractDeployCommit(r.Description)
			}
			msg := strings.TrimSpace(r.Description)
			if msg == "" {
				msg = fmt.Sprintf("Release %s (%s)", ver, r.Status)
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: "release:" + r.ID,
				Platform:   "heroku",
				Status:     status,
				Version:    ver,
				CommitSHA:  commitSHA,
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
	app := core.Str(target, "app_name")
	cli := fmt.Sprintf(
		`heroku webhooks:add -i api:build,api:release -l notify -s '%s' -u '%s' -a '%s'`,
		secret, webhookURL, app,
	)
	return core.WebhookStatus{
		Plugin:      "heroku",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("Heroku Dashboard > App %s > More > View webhooks (or use Heroku CLI). Events: api:build, api:release. Level: notify. URL: %s. Secret: %s.", app, webhookURL, secret),
	}
}

// ConnectWebhook creates (or reuses) a Heroku App Webhook subscribed to api:build and api:release.
func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	app := strings.TrimSpace(core.Str(target, "app_name"))
	if app == "" {
		return "", errors.New("app_name is required")
	}
	hdr := headers(core.Str(cfg, "api_token"), nil)
	hookURL := fmt.Sprintf("%s/apps/%s/webhooks", baseURL, url.PathEscape(app))

	var existing []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, hookURL, hdr, &existing); err == nil {
		for _, h := range existing {
			if h.URL == webhookURL {
				return h.ID, nil
			}
		}
	}
	payload := map[string]any{
		"include": []string{"api:build", "api:release"},
		"level":   "notify",
		"secret":  secret,
		"url":     webhookURL,
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, hookURL, hdr, payload, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}
