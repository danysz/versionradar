// Package supabase tracks Supabase database migrations, Edge Functions versions, and project status
// via the Supabase Management API.
package supabase

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.Stub }

func (Plugin) Name() string    { return "supabase" }
func (Plugin) Version() string { return "1.0.0" }
func (Plugin) Tier() string    { return core.TierCommunity }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:    "access_token",
			Label:  "Supabase Access Token",
			Help:   "Supabase Dashboard > Account > Access Tokens (read-only Management API token)",
			Secret: true,
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project_ref", Label: "Project Ref", Help: "20-char project reference ID from your Supabase URL"},
		{
			Key:      "resource",
			Label:    "What to track",
			Default:  "migrations",
			Options:  []string{"migrations", "functions", "project"},
			Help:     "Track applied DB migrations, Edge Functions versions, or project DB status",
			Optional: true,
		},
		{Key: "function_slug", Label: "Edge Function slug (optional)", Help: "Only used when tracking functions (leave empty for all)", Optional: true},
	}
}

var baseURL = "https://api.supabase.com/v1"

func headers(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + strings.TrimSpace(tok)}
}

func parseMigrationTime(ver string) time.Time {
	// Supabase migration versions typically start with YYYYMMDDHHMMSS
	if len(ver) >= 14 {
		if ts, err := time.Parse("20060102150405", ver[:14]); err == nil {
			return ts
		}
	}
	return time.Time{}
}

func mapProjectStatus(s string) (status, stage string, progress int) {
	switch strings.ToUpper(s) {
	case "ACTIVE_HEALTHY":
		return core.StatusSuccess, "healthy", 100
	case "ACTIVE_UNHEALTHY", "INIT_FAILED", "RESTORE_FAILED", "PAUSE_FAILED":
		return core.StatusFailed, strings.ToLower(s), 100
	case "COMING_UP", "RESTORING", "UPGRADING", "PAUSING", "GOING_DOWN":
		return core.StatusInProgress, strings.ToLower(s), 55
	default:
		return core.StatusInProgress, strings.ToLower(s), 50
	}
}

func (Plugin) CheckStatus(ctx context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	hdr := headers(core.Str(cfg, "access_token"))
	var out []core.DeploymentEvent

	for _, t := range core.Targets(cfg) {
		ref := strings.TrimSpace(core.Str(t, "project_ref"))
		if ref == "" {
			continue
		}
		res := strings.ToLower(strings.TrimSpace(core.Str(t, "resource")))
		if res == "" {
			res = "migrations"
		}
		pid := core.Str(t, "product")

		switch res {
		case "migrations":
			var migs []struct {
				Version string `json:"version"`
				Name    string `json:"name"`
			}
			u := fmt.Sprintf("%s/projects/%s/database/migrations", baseURL, url.PathEscape(ref))
			if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &migs); err != nil {
				return out, err
			}
			// Sort newest migration first and take top 10.
			sort.Slice(migs, func(i, j int) bool { return migs[i].Version > migs[j].Version })
			if len(migs) > 10 {
				migs = migs[:10]
			}
			for _, m := range migs {
				msg := m.Name
				if msg == "" {
					msg = "DB migration " + m.Version
				}
				out = append(out, core.DeploymentEvent{
					ProductID:  pid,
					ExternalID: "mig:" + m.Version,
					Platform:   "supabase",
					Status:     core.StatusSuccess,
					Version:    m.Version,
					Stage:      "migrated",
					Progress:   100,
					Message:    msg,
					Timestamp:  parseMigrationTime(m.Version),
				})
			}

		case "functions":
			var fns []struct {
				ID        string `json:"id"`
				Slug      string `json:"slug"`
				Name      string `json:"name"`
				Status    string `json:"status"`
				Version   int    `json:"version"`
				UpdatedAt int64  `json:"updated_at"`
			}
			u := fmt.Sprintf("%s/projects/%s/functions", baseURL, url.PathEscape(ref))
			if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &fns); err != nil {
				return out, err
			}
			filterSlug := strings.TrimSpace(core.Str(t, "function_slug"))
			for _, f := range fns {
				if filterSlug != "" && f.Slug != filterSlug {
					continue
				}
				st := core.StatusSuccess
				if !strings.EqualFold(f.Status, "ACTIVE") {
					st = core.StatusFailed
				}
				var ts time.Time
				if f.UpdatedAt > 0 {
					ts = time.UnixMilli(f.UpdatedAt)
				}
				out = append(out, core.DeploymentEvent{
					ProductID:  pid,
					ExternalID: fmt.Sprintf("fn:%s:v%d", f.Slug, f.Version),
					Platform:   "supabase",
					Status:     st,
					Version:    fmt.Sprintf("v%d", f.Version),
					Stage:      strings.ToLower(f.Status),
					Progress:   100,
					Message:    fmt.Sprintf("Edge function %s", f.Slug),
					Timestamp:  ts,
				})
			}

		case "project":
			var proj struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Status   string `json:"status"`
				Region   string `json:"region"`
				Database struct {
					Version string `json:"version"`
				} `json:"database"`
			}
			u := fmt.Sprintf("%s/projects/%s", baseURL, url.PathEscape(ref))
			if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, hdr, &proj); err != nil {
				return out, err
			}
			st, stage, prog := mapProjectStatus(proj.Status)
			ver := proj.Database.Version
			if ver == "" {
				ver = proj.Status
			}
			out = append(out, core.DeploymentEvent{
				ProductID:  pid,
				ExternalID: "proj:" + ref + ":" + ver,
				Platform:   "supabase",
				Status:     st,
				Version:    ver,
				Stage:      stage,
				Progress:   prog,
				Message:    fmt.Sprintf("%s (%s)", proj.Name, proj.Region),
			})
		}
	}
	return out, nil
}

func (Plugin) WebhookPlan(_ string, _ string, _ map[string]any) core.WebhookStatus {
	return core.WebhookStatus{
		Plugin:      "supabase",
		Kind:        "provider",
		Supported:   false,
		Automatable: false,
		Note:        "Supabase Management API does not emit schema migration webhooks. VersionRadar polls migrations, Edge Functions, and DB status automatically every 5 minutes and on Sync.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("supabase plugin is poll-only")
}
