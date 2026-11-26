// Package firebase reads releases from Firebase App Distribution (poll only: it has no webhooks).
package firebase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "firebase" }
func (Plugin) Version() string { return "1.1.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "service_account_json", Label: "Service account key (JSON)", Help: "Role: Firebase App Distribution Viewer only", Secret: true, Multiline: true, JSON: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project_id", Label: "Firebase project id"},
		{Key: "app_id", Label: "Firebase app id", Help: "e.g. 1:1234567890:android:abc123"},
	}
}

var baseURL = "https://firebaseappdistribution.googleapis.com/v1"

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hc, err := core.GoogleClient(ctx, cfg, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		var resp struct {
			Releases []struct {
				Name           string `json:"name"`
				DisplayVersion string `json:"displayVersion"`
				BuildVersion   string `json:"buildVersion"`
				CreateTime     string `json:"createTime"`
				Notes          struct {
					Text string `json:"text"`
				} `json:"releaseNotes"`
			} `json:"releases"`
		}
		u := fmt.Sprintf("%s/projects/%s/apps/%s/releases?pageSize=10", baseURL,
			url.PathEscape(core.Str(t, "project_id")), url.PathEscape(core.Str(t, "app_id")))
		if err := core.Do(ctx, hc, http.MethodGet, u, nil, &resp); err != nil {
			return out, err
		}
		for _, r := range resp.Releases {
			ts, _ := time.Parse(time.RFC3339, r.CreateTime)
			out = append(out, core.DeploymentEvent{
				ProductID:  core.Str(t, "product"),
				ExternalID: path.Base(r.Name),
				Platform:   "firebase",
				Status:     core.StatusSuccess,
				Version:    fmt.Sprintf("%s (%s)", r.DisplayVersion, r.BuildVersion),
				Stage:      "distributed",
				Progress:   100,
				Message:    r.Notes.Text,
				Timestamp:  ts,
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(_ string, _ string, _ map[string]any) core.WebhookStatus {
	return core.WebhookStatus{
		Plugin:      "firebase",
		Kind:        "provider",
		Supported:   false,
		Automatable: false,
		Note:        "Firebase App Distribution does not offer release webhooks. VersionRadar polls it automatically every 5 minutes and on manual sync.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("firebase app distribution does not support webhooks (poll-only)")
}
