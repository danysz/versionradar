// Package github integrates GitHub repositories as a VersionRadar code source (API + push/tag webhooks).
package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func (Plugin) Name() string    { return "github" }
func (Plugin) Version() string { return "1.0.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{
			Key:      "token",
			Label:    "Personal Access Token",
			Help:     "Fine-grained token with Contents: Read (add Webhooks: Write for 1-click webhook connect; optional for public repos)",
			Secret:   true,
			Optional: true,
		},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "repo", Label: "Repository (owner/name)", Help: "e.g. octocat/hello-world"},
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
	repo = strings.TrimSpace(strings.TrimSuffix(repo, ".git"))
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("repo must be in owner/name format")
	}
	return parts[0], parts[1], nil
}

type repoClient struct {
	owner, name string
	hdr         map[string]string
}

func (Plugin) OpenRepo(_ context.Context, cfg, target map[string]any) (core.RepoClient, error) {
	owner, name, err := splitRepo(core.Str(target, "repo"))
	if err != nil {
		return nil, err
	}
	return &repoClient{owner: owner, name: name, hdr: headers(core.Str(cfg, "token"))}, nil
}

func (r *repoClient) ListTags(ctx context.Context) ([]core.GitTag, error) {
	var raw []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	u := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=100", baseURL, url.PathEscape(r.owner), url.PathEscape(r.name))
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, r.hdr, &raw); err != nil {
		return nil, err
	}
	out := make([]core.GitTag, 0, len(raw))
	for _, t := range raw {
		out = append(out, core.GitTag{Name: t.Name, CommitSHA: t.Commit.SHA})
	}
	return out, nil
}

func (r *repoClient) BranchHead(ctx context.Context, branch string) (core.GitCommit, error) {
	var raw struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", baseURL, url.PathEscape(r.owner), url.PathEscape(r.name), url.PathEscape(branch))
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, r.hdr, &raw); err != nil {
		return core.GitCommit{}, err
	}
	ts, _ := time.Parse(time.RFC3339, raw.Commit.Committer.Date)
	return core.GitCommit{SHA: raw.SHA, Message: raw.Commit.Message, Date: ts}, nil
}

func (r *repoClient) ReadFile(ctx context.Context, ref, filePath string) ([]byte, error) {
	var raw struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	cleanPath := strings.TrimPrefix(strings.TrimSpace(filePath), "/")
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", baseURL, url.PathEscape(r.owner), url.PathEscape(r.name), cleanPath, url.QueryEscape(ref))
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, u, r.hdr, &raw); err != nil {
		return nil, err
	}
	if raw.Encoding == "base64" {
		cleaned := strings.ReplaceAll(raw.Content, "\n", "")
		return base64.StdEncoding.DecodeString(cleaned)
	}
	return []byte(raw.Content), nil
}

// VerifyWebhook checks GitHub's X-Hub-Signature-256 header (sha256=<hex>).
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

// ParsePushWebhook parses push, create (tag), or release payloads.
func (Plugin) ParsePushWebhook(body []byte) (string, string, error) {
	var p struct {
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Release struct {
			TagName string `json:"tag_name"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return "", "", err
	}
	ref := p.Ref
	if ref == "" && p.Release.TagName != "" {
		ref = "refs/tags/" + p.Release.TagName
	}
	return ref, p.After, nil
}

func (Plugin) WebhookPlan(webhookURL, secret string, target map[string]any) core.WebhookStatus {
	repo := core.Str(target, "repo")
	cli := fmt.Sprintf(
		`gh api repos/%s/hooks --method POST -f name=web -F active=true -f 'events[]=push' -f 'events[]=create' -f 'events[]=release' -f 'config[url]=%s' -f 'config[content_type]=json' -f 'config[secret]=%s'`,
		repo, webhookURL, secret,
	)
	return core.WebhookStatus{
		Plugin:      "github",
		Kind:        "source",
		Supported:   true,
		Automatable: true,
		WebhookURL:  webhookURL,
		CLICommand:  cli,
		ManualSteps: fmt.Sprintf("GitHub repo %s > Settings > Webhooks > Add webhook. Payload URL: %s, Content type: application/json, Secret: %s, Events: Pushes + Branch or tag creation + Releases.", repo, webhookURL, secret),
	}
}

func (Plugin) ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error) {
	owner, name, err := splitRepo(core.Str(target, "repo"))
	if err != nil {
		return "", err
	}
	tok := core.Str(cfg, "token")
	if tok == "" {
		return "", errors.New("GitHub token with Webhooks: Write permission is required to auto-connect")
	}
	hdr := headers(tok)
	listURL := fmt.Sprintf("%s/repos/%s/%s/hooks?per_page=100", baseURL, url.PathEscape(owner), url.PathEscape(name))
	var existing []struct {
		ID     int64 `json:"id"`
		Config struct {
			URL string `json:"url"`
		} `json:"config"`
	}
	if err := core.Do(ctx, core.DefaultClient, http.MethodGet, listURL, hdr, &existing); err == nil {
		for _, h := range existing {
			if h.Config.URL == webhookURL {
				return fmt.Sprintf("%d", h.ID), nil
			}
		}
	}
	payload := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push", "create", "release"},
		"config": map[string]any{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
	}
	var created struct {
		ID int64 `json:"id"`
	}
	createURL := fmt.Sprintf("%s/repos/%s/%s/hooks", baseURL, url.PathEscape(owner), url.PathEscape(name))
	if err := core.DoWithBody(ctx, core.DefaultClient, http.MethodPost, createURL, hdr, payload, &created); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", created.ID), nil
}
