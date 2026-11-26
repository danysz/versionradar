// Package gitlab integrates GitLab repositories (gitlab.com or self-hosted) as a VersionRadar code source.
package gitlab

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

func (Plugin) Name() string    { return "gitlab" }
func (Plugin) Version() string { return "1.0.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:      "token",
			Label:    "Access Token",
			Help:     "Project or Personal Access Token with read_repository/read_api (use api scope for 1-click webhook connect; optional for public repos)",
			Secret:   true,
			Optional: true,
		},
		{
			Key:      "base_url",
			Label:    "GitLab Base URL",
			Help:     "Leave empty for https://gitlab.com",
			Optional: true,
			Default:  "https://gitlab.com",
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "project", Label: "Project path or ID", Help: "e.g. group/subgroup/repo or numeric project ID"},
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

func headers(token string) map[string]string {
	h := map[string]string{"User-Agent": "VersionRadar"}
	if strings.TrimSpace(token) != "" {
		h["PRIVATE-TOKEN"] = strings.TrimSpace(token)
	}
	return h
}

type repoClient struct {
	base    string
	project string
	hdr     map[string]string
}

func (Plugin) OpenRepo(_ context.Context, cfg, target map[string]any) (core.RepoClient, error) {
	proj := strings.TrimSpace(strings.TrimSuffix(core.Str(target, "project"), ".git"))
	if proj == "" {
		return nil, errors.New("GitLab project path or ID is required")
	}
	return &repoClient{base: apiBase(cfg), project: proj, hdr: headers(core.Str(cfg, "token"))}, nil
}

func (r *repoClient) ListTags(ctx context.Context) ([]core.GitTag, error) {
	var raw []struct {
		Name   string `json:"name"`
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	u := fmt.Sprintf("%s/projects/%s/repository/tags?per_page=100", r.base, url.PathEscape(r.project))
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, r.hdr, &raw); err != nil {
		return nil, err
	}
	out := make([]core.GitTag, 0, len(raw))
	for _, t := range raw {
		out = append(out, core.GitTag{Name: t.Name, CommitSHA: t.Commit.ID})
	}
	return out, nil
}

func (r *repoClient) BranchHead(ctx context.Context, branch string) (core.GitCommit, error) {
	var raw struct {
		ID            string `json:"id"`
		Message       string `json:"message"`
		CommittedDate string `json:"committed_date"`
	}
	u := fmt.Sprintf("%s/projects/%s/repository/commits/%s", r.base, url.PathEscape(r.project), url.PathEscape(branch))
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, r.hdr, &raw); err != nil {
		return core.GitCommit{}, err
	}
	ts, _ := time.Parse(time.RFC3339, raw.CommittedDate)
	return core.GitCommit{SHA: raw.ID, Message: raw.Message, Date: ts}, nil
}

func (r *repoClient) ReadFile(ctx context.Context, ref, filePath string) ([]byte, error) {
	cleanPath := strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	u := fmt.Sprintf("%s/projects/%s/repository/files/%s/raw?ref=%s", r.base, url.PathEscape(r.project), url.PathEscape(cleanPath), url.QueryEscape(ref))
	return core.DoRaw(ctx, core.DefaultClient, http.MethodGet, u, r.hdr)
}

func (Plugin) VerifyWebhook(r *http.Request, _ []byte, secret string) error {
	if secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Gitlab-Token")), []byte(secret)) != 1 {
		return errors.New("bad X-Gitlab-Token")
	}
	return nil
}

func (Plugin) ParsePushWebhook(body []byte) (string, string, error) {
	var p struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", "", err
	}
	return p.Ref, p.After, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	proj := core.Str(target, "project")
	cli := fmt.Sprintf(
		`curl -X POST "https://gitlab.com/api/v4/projects/%s/hooks" -H "PRIVATE-TOKEN: $GITLAB_TOKEN" -H "Content-Type: application/json" -d '{"url":"%s","token":"%s","push_events":true,"tag_push_events":true,"releases_events":true}'`,
		url.PathEscape(proj), webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "gitlab",
		Kind:        "source",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("GitLab project %s > Settings > Webhooks > Add new webhook. URL: %s, Secret token: %s, Triggers: Push events + Tag push events + Releases events.", proj, webhookURL, secret),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	proj := strings.TrimSpace(strings.TrimSuffix(core.Str(target, "project"), ".git"))
	if proj == "" {
		return "", errors.New("GitLab project is required")
	}
	tok := core.Str(cfg, "token")
	if tok == "" {
		return "", errors.New("GitLab token with api scope is required to auto-connect")
	}
	base := apiBase(cfg)
	hdr := headers(tok)
	listURL := fmt.Sprintf("%s/projects/%s/hooks", base, url.PathEscape(proj))
	var existing []struct {
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, listURL, hdr, &existing); err == nil {
		for _, h := range existing {
			if h.URL == webhookURL {
				return fmt.Sprintf("%d", h.ID), nil
			}
		}
	}
	payload := map[string]any{
		"url":             webhookURL,
		"token":           secret,
		"push_events":     true,
		"tag_push_events": true,
		"releases_events": true,
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, listURL, hdr, payload, &created); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", created.ID), nil
}
