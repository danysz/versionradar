// Package googleplay reads release tracks from the Google Play Developer API (poll only: Play has no release webhooks).
package googleplay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "googleplay" }
func (Plugin) Version() string { return "1.1.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "service_account_json", Label: "Service account key (JSON)", Help: "Invited in Play Console with read-only access", Secret: true, Multiline: true, JSON: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "package_name", Label: "Package name", Help: "e.g. com.example.app"},
	}
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hc, err := core.GoogleClient(ctx, cfg, "https://www.googleapis.com/auth/androidpublisher")
	if err != nil {
		return nil, err
	}
	var out []core.DeploymentEvent
	for _, t := range core.Targets(cfg) {
		base := "https://androidpublisher.googleapis.com/androidpublisher/v3/applications/" + url.PathEscape(core.Str(t, "package_name")) + "/edits"
		var edit struct {
			ID string `json:"id"`
		}
		if err := core.Do(ctx, hc, http.MethodPost, base, nil, &edit); err != nil {
			return out, err
		}
		var resp struct {
			Tracks []struct {
				Track    string `json:"track"`
				Releases []struct {
					Name         string   `json:"name"`
					Status       string   `json:"status"`
					VersionCodes []string `json:"versionCodes"`
				} `json:"releases"`
			} `json:"tracks"`
		}
		err := core.Do(ctx, hc, http.MethodGet, fmt.Sprintf("%s/%s/tracks", base, edit.ID), nil, &resp)
		core.Do(ctx, hc, http.MethodDelete, fmt.Sprintf("%s/%s", base, edit.ID), nil, nil)
		if err != nil {
			return out, err
		}
		for _, tr := range resp.Tracks {
			for _, r := range tr.Releases {
				code := ""
				if len(r.VersionCodes) > 0 {
					code = r.VersionCodes[0]
				}
				status := core.StatusInProgress
				prog := 50
				switch r.Status {
				case "completed":
					status = core.StatusSuccess
					prog = 100
				case "halted":
					status = core.StatusFailed
					prog = 100
				case "draft":
					prog = 20
				}
				v := r.Name
				if v == "" {
					v = code
				}
				out = append(out, core.DeploymentEvent{
					ProductID:  core.Str(t, "product"),
					ExternalID: tr.Track + ":" + code,
					Platform:   "googleplay",
					Status:     status,
					Version:    v,
					Stage:      tr.Track + ":" + r.Status,
					Progress:   prog,
					Message:    tr.Track + " " + r.Status,
				})
			}
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(_ string, _ string, _ map[string]any) core.WebhookStatus {
	return core.WebhookStatus{
		Plugin:      "googleplay",
		Kind:        "provider",
		Supported:   false,
		Automatable: false,
		Note:        "Google Play does not offer release status webhooks. VersionRadar polls release tracks automatically every 5 minutes and on manual sync.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("google play does not support release webhooks (poll-only)")
}
