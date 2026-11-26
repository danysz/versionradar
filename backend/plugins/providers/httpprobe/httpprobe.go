// Package httpprobe polls any HTTP/HTTPS /version or /health endpoint (and accepts generic CI push webhooks),
// allowing VersionRadar to track any self-hosted server, Kubernetes service, or custom app.
package httpprobe

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{}

func (Plugin) Name() string    { return "http_probe" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:      "auth_header",
			Label:    "Authorization Header (optional)",
			Help:     "Optional header value for private endpoints, e.g. Bearer my-token",
			Secret:   true,
			Optional: true,
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "url", Label: "Endpoint URL", Help: "e.g. https://api.example.com/version or /health"},
		{Key: "json_path", Label: "JSON version path (optional)", Help: "Dot path in JSON response, e.g. version or build.version", Optional: true},
		{Key: "commit_path", Label: "JSON commit SHA path (optional)", Help: "Optional dot path for commit SHA, e.g. commit or git.sha", Optional: true},
		{Key: "regex", Label: "Regex extract (optional)", Help: `Optional regex with capture group if response is plain text/HTML`, Optional: true},
	}
}

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
		Stage     string `json:"stage"`
		Progress  int    `json:"progress"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	status := core.StatusSuccess
	switch strings.ToLower(p.Status) {
	case "failed", "failure", "error":
		status = core.StatusFailed
	case "in_progress", "building", "deploying", "pending", "started":
		status = core.StatusInProgress
	}
	extID := p.ID
	if extID == "" {
		extID = "push:" + p.Version + ":" + p.CommitSHA
	}
	prog := p.Progress
	if prog == 0 && status != core.StatusInProgress {
		prog = 100
	}
	return &core.DeploymentEvent{
		ExternalID: extID,
		Platform:   "http_probe",
		Status:     status,
		Version:    p.Version,
		CommitSHA:  p.CommitSHA,
		Stage:      p.Stage,
		Progress:   prog,
		Message:    p.Message,
	}, nil
}

func lookupDotPath(obj any, path string) string {
	cur := obj
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = m[seg]
		if !ok {
			return ""
		}
	}
	switch v := cur.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%g", v)
	default:
		return ""
	}
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := map[string]string{"User-Agent": "VersionRadar-HTTPProbe/1.0"}
	if auth := strings.TrimSpace(core.Str(cfg, "auth_header")); auth != "" {
		hdr["Authorization"] = auth
	}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		endpoint := strings.TrimSpace(core.Str(t, "url"))
		if endpoint == "" {
			continue
		}
		raw, err := core.DoRaw(ctx, core.DefaultClient, http.MethodGet, endpoint, hdr)
		if err != nil {
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: "probe:" + endpoint + ":err",
				Platform:   "http_probe",
				Status:     core.StatusFailed,
				Stage:      "unreachable",
				Progress:   100,
				Message:    err.Error(),
			})
			continue
		}

		ver := ""
		commit := ""
		jsonPath := strings.TrimSpace(core.Str(t, "json_path"))
		commitPath := strings.TrimSpace(core.Str(t, "commit_path"))
		reStr := strings.TrimSpace(core.Str(t, "regex"))

		if jsonPath != "" || commitPath != "" {
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return out, fmt.Errorf("endpoint %s did not return JSON: %w", endpoint, err)
			}
			if jsonPath != "" {
				ver = lookupDotPath(parsed, jsonPath)
			}
			if commitPath != "" {
				commit = lookupDotPath(parsed, commitPath)
			}
		} else if reStr != "" {
			re, err := regexp.Compile(reStr)
			if err != nil {
				return out, err
			}
			if m := re.FindStringSubmatch(string(raw)); len(m) >= 2 {
				ver = strings.TrimSpace(m[1])
			} else if len(m) == 1 {
				ver = strings.TrimSpace(m[0])
			}
		} else {
			// Auto-detect JSON {"version": ..., "commit": ...} or plain text first line.
			var obj map[string]any
			if json.Unmarshal(raw, &obj) == nil {
				for _, k := range []string{"version", "release", "tag", "app_version"} {
					if v := lookupDotPath(obj, k); v != "" {
						ver = v
						break
					}
				}
				for _, k := range []string{"commit", "commit_sha", "sha", "git_sha"} {
					if c := lookupDotPath(obj, k); c != "" {
						commit = c
						break
					}
				}
			}
			if ver == "" {
				ver = strings.TrimSpace(strings.Split(string(raw), "\n")[0])
			}
		}

		if ver == "" && commit != "" {
			ver = core.ShortSHA(commit, "")
		}
		out = append(out, core.DeploymentEvent{
			ProductID:  core.Str(t, "product"),
			ExternalID: "probe:" + ver + ":" + commit,
			Platform:   "http_probe",
			Status:     core.StatusSuccess,
			Version:    ver,
			CommitSHA:  commit,
			Stage:      "live",
			Progress:   100,
			Message:    "HTTP 200 OK",
		})
	}
	return out, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, _ map[string]any) core.WebhookStatus {
	cli := fmt.Sprintf(
		`curl -X POST '%s' -H 'X-VersionRadar-Secret: %s' -H 'Content-Type: application/json' -d '{"version":"1.2.3","commit_sha":"abcdef1","status":"success","stage":"deployed","message":"CI deploy complete"}'`,
		webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "http_probe",
		Kind:        "provider",
		Supported:   true,
		Automatable: false,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: "Call this endpoint from your CI/CD pipeline (on build start, finish, or failure) with header X-VersionRadar-Secret.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("http_probe uses a custom CI curl call; copy the CLI command from Manual / CLI")
}
