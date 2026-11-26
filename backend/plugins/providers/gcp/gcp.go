// Package gcp handles Cloud Build polling, Pub/Sub push notifications, and webhook subscription automation.
package gcp

import (
	"context"
	"crypto/subtle"
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

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "gcp" }
func (Plugin) Version() string { return "1.1.0" }

// Pub/Sub push carries the shared secret in ?token= on the push endpoint.
func (Plugin) VerifyWebhook(r *http.Request, _ []byte, secret string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("token")), []byte(secret)) != 1 {
		return errors.New("bad token")
	}
	return nil
}

type buildStep struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (Plugin) ParseWebhook(body []byte, _ map[string]any) (*core.DeploymentEvent, error) {
	var env struct {
		Message struct {
			Data string `json:"data"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(env.Message.Data)
	if err != nil {
		return nil, err
	}
	var b struct {
		ID            string            `json:"id"`
		Status        string            `json:"status"`
		CreateTime    string            `json:"createTime"`
		Substitutions map[string]string `json:"substitutions"`
		Steps         []buildStep       `json:"steps"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	stage, prog := buildProgress(b.Status, b.Steps)
	ts, _ := time.Parse(time.RFC3339, b.CreateTime)
	return &core.DeploymentEvent{
		Platform:   "gcp",
		Status:     mapStatus(b.Status),
		Version:    version(b.ID, b.Substitutions),
		CommitSHA:  commitSHA(b.Substitutions),
		ExternalID: b.ID,
		Stage:      stage,
		Progress:   prog,
		Message:    buildMessage(b.Status, b.Steps),
		Timestamp:  ts,
	}, nil
}

func mapStatus(s string) string {
	switch s {
	case "SUCCESS":
		return core.StatusSuccess
	case "FAILURE", "INTERNAL_ERROR", "TIMEOUT", "CANCELLED", "EXPIRED":
		return core.StatusFailed
	}
	return core.StatusInProgress
}

func buildProgress(status string, steps []buildStep) (string, int) {
	switch status {
	case "SUCCESS":
		return "completed", 100
	case "FAILURE", "INTERNAL_ERROR", "TIMEOUT", "CANCELLED", "EXPIRED":
		return strings.ToLower(status), 100
	case "QUEUED", "PENDING":
		return "queued", 15
	}
	if len(steps) > 0 {
		done := 0
		activeName := "working"
		for _, st := range steps {
			if st.Status == "SUCCESS" {
				done++
			} else if st.Status == "WORKING" {
				if st.ID != "" {
					activeName = st.ID
				} else {
					activeName = "step"
				}
			}
		}
		pct := 20 + (done*70)/len(steps)
		return fmt.Sprintf("%s (%d/%d)", activeName, done, len(steps)), pct
	}
	return "working", 55
}

func buildMessage(status string, steps []buildStep) string {
	if status == "WORKING" && len(steps) > 0 {
		done := 0
		for _, st := range steps {
			if st.Status == "SUCCESS" {
				done++
			}
		}
		return fmt.Sprintf("Cloud Build WORKING (%d/%d steps)", done, len(steps))
	}
	return "Cloud Build " + status
}

func version(id string, subs map[string]string) string {
	for _, k := range []string{"TAG_NAME", "SHORT_SHA"} {
		if subs[k] != "" {
			return subs[k]
		}
	}
	if sha := subs["COMMIT_SHA"]; sha != "" {
		return core.ShortSHA(sha, id)
	}
	return id
}

func commitSHA(subs map[string]string) string {
	if s := subs["COMMIT_SHA"]; s != "" {
		return s
	}
	return subs["SHORT_SHA"]
}

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "service_account_json", Label: "Service account key (JSON)", Help: "Role: Cloud Build Viewer (add Pub/Sub Editor for 1-click webhook connect)", Secret: true, Multiline: true, JSON: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project_id", Label: "GCP project id"},
	}
}

var (
	cloudBuildBase = "https://cloudbuild.googleapis.com/v1"
	pubsubBase     = "https://pubsub.googleapis.com/v1"
)

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hc, err := core.GoogleClient(ctx, cfg, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		var resp struct {
			Builds []struct {
				ID            string            `json:"id"`
				Status        string            `json:"status"`
				CreateTime    string            `json:"createTime"`
				Substitutions map[string]string `json:"substitutions"`
				Steps         []buildStep       `json:"steps"`
			} `json:"builds"`
		}
		u := fmt.Sprintf("%s/projects/%s/builds?pageSize=10", cloudBuildBase, url.PathEscape(core.Str(t, "project_id")))
		if err := core.Do(ctx, hc, http.MethodGet, u, nil, &resp); err != nil {
			return out, err
		}
		for _, b := range resp.Builds {
			ts, _ := time.Parse(time.RFC3339, b.CreateTime)
			stage, prog := buildProgress(b.Status, b.Steps)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: b.ID,
				Platform:   "gcp",
				Status:     mapStatus(b.Status),
				Version:    version(b.ID, b.Substitutions),
				CommitSHA:  commitSHA(b.Substitutions),
				Stage:      stage,
				Progress:   prog,
				Message:    buildMessage(b.Status, b.Steps),
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func pushEndpoint(webhookURL, secret string) string {
	sep := "?"
	if strings.Contains(webhookURL, "?") {
		sep = "&"
	}
	return webhookURL + sep + "token=" + url.QueryEscape(secret)
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	proj := core.Str(target, "project_id")
	subName := "versionradar-" + core.ShortSHA(core.Str(target, "product"), "hook")
	endpoint := pushEndpoint(webhookURL, secret)
	cli := fmt.Sprintf(
		`gcloud pubsub topics create cloud-builds --project=%s 2>/dev/null || true; gcloud pubsub subscriptions create %s --topic=cloud-builds --push-endpoint='%s' --project=%s`,
		proj, subName, endpoint, proj,
	)
	return core.WebhookStatus{
		Plugin:      "gcp",
		Kind:        "provider",
		Supported:   true,
		Automatable: true,
		WebhookURL:  endpoint,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("GCP Console > Pub/Sub > Topic `cloud-builds` (in project %s) > Create Push Subscription with endpoint: %s. Cloud Build publishes QUEUED, WORKING, SUCCESS, and FAILURE events automatically.", proj, endpoint),
	}
}

// ConnectWebhook creates the Pub/Sub push subscription on `cloud-builds` for instant build start/progress/finish events.
func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	proj := strings.TrimSpace(core.Str(target, "project_id"))
	if proj == "" {
		return "", errors.New("project_id is required")
	}
	hc, err := core.GoogleClient(ctx, cfg, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", err
	}
	topic := fmt.Sprintf("projects/%s/topics/cloud-builds", url.PathEscape(proj))
	// Ensure topic exists (409 ALREADY_EXISTS is fine).
	_ = core.DoWithBody(ctx, hc, http.MethodPut, fmt.Sprintf("%s/%s", pubsubBase, topic), nil, map[string]any{}, nil)

	subName := "versionradar-" + core.ShortSHA(core.Str(target, "product"), "hook")
	subPath := fmt.Sprintf("projects/%s/subscriptions/%s", url.PathEscape(proj), url.PathEscape(subName))
	endpoint := pushEndpoint(webhookURL, secret)
	payload := map[string]any{
		"topic": topic,
		"pushConfig": map[string]any{
			"pushEndpoint": endpoint,
		},
		"ackDeadlineSeconds": 30,
	}
	if err := core.DoWithBody(ctx, hc, http.MethodPut, fmt.Sprintf("%s/%s", pubsubBase, subPath), nil, payload, nil); err != nil {
		// If subscription already exists (409), update its pushConfig so reconnect is idempotent.
		if strings.Contains(err.Error(), "HTTP 409") {
			modURL := fmt.Sprintf("%s/%s:modifyPushConfig", pubsubBase, subPath)
			if merr := core.DoWithBody(ctx, hc, http.MethodPost, modURL, nil, map[string]any{
				"pushConfig": map[string]any{"pushEndpoint": endpoint},
			}, nil); merr != nil {
				return "", fmt.Errorf("update pubsub push config: %w", merr)
			}
			return subPath, nil
		}
		return "", fmt.Errorf("create pubsub push subscription (needs Pub/Sub Editor role): %w", err)
	}
	return subPath, nil
}
