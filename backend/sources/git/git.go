// Package git reads refs (tags and branch heads) from any HTTPS Git repository using the Git Smart HTTP protocol
// (equivalent to `git ls-remote`, with zero external binaries required).
package git

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/danysz/versionradar/backend/core"
)

type Plugin struct{ core.SourceStub }

func (Plugin) Name() string    { return "git" }
func (Plugin) Version() string { return "1.0.0" }

func (Plugin) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "username", Label: "Username (optional)", Help: "For private HTTPS repos (e.g. git, oauth2, or your username)", Optional: true},
		{Key: "token", Label: "Password / Access Token (optional)", Help: "Read-only PAT for private repos; leave empty for public repos", Secret: true, Optional: true},
	}
}

func (Plugin) TargetFields() []core.Field {
	return []core.Field{
		{Key: "repo_url", Label: "Git Repository HTTPS URL", Help: "e.g. https://git.example.com/org/app.git"},
		{Key: "raw_file_url", Label: "Raw file URL template (optional)", Help: "Only needed for git_file strategy, e.g. https://git.example.com/org/app/raw/{ref}/{path}", Optional: true},
	}
}

type repoClient struct {
	repoURL    string
	rawFileURL string
	hdr        map[string]string
}

func (Plugin) OpenRepo(_ context.Context, cfg, target map[string]any) (core.RepoClient, error) {
	u := strings.TrimSpace(core.Str(target, "repo_url"))
	if u == "" {
		return nil, errors.New("repo_url is required")
	}
	u = strings.TrimRight(u, "/")
	hdr := map[string]string{"User-Agent": "git/2.40.0"}
	tok := strings.TrimSpace(core.Str(cfg, "token"))
	user := strings.TrimSpace(core.Str(cfg, "username"))
	if tok != "" {
		if user == "" {
			user = "oauth2"
		}
		hdr["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+tok))
	}
	return &repoClient{
		repoURL:    u,
		rawFileURL: strings.TrimSpace(core.Str(target, "raw_file_url")),
		hdr:        hdr,
	}, nil
}

func (r *repoClient) fetchRefs(ctx context.Context) (map[string]string, error) {
	u := r.repoURL + "/info/refs?service=git-upload-pack"
	raw, err := core.DoRaw(ctx, core.DefaultClient, http.MethodGet, u, r.hdr)
	if err != nil {
		return nil, err
	}
	return parsePktLines(raw), nil
}

// parsePktLines decodes Git Smart HTTP pkt-line responses (or dumb HTTP tab-separated info/refs).
func parsePktLines(raw []byte) map[string]string {
	refs := map[string]string{}
	s := string(raw)
	if len(s) >= 4 {
		if _, err := strconv.ParseInt(s[:4], 16, 32); err == nil {
			for i := 0; i+4 <= len(s); {
				n, err := strconv.ParseInt(s[i:i+4], 16, 32)
				if err != nil {
					break
				}
				if n == 0 {
					i += 4
					continue
				}
				if n < 4 || i+int(n) > len(s) {
					break
				}
				line := strings.TrimRight(s[i+4:i+int(n)], "\r\n")
				i += int(n)
				if strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.IndexByte(line, 0); idx >= 0 {
					line = line[:idx]
				}
				if sha, ref, ok := strings.Cut(line, " "); ok && len(sha) >= 7 {
					refs[strings.TrimSpace(ref)] = strings.TrimSpace(sha)
				}
			}
			return refs
		}
	}
	// Fallback: dumb HTTP info/refs (sha\tref\n)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if sha, ref, ok := strings.Cut(line, "\t"); ok && len(sha) >= 7 {
			refs[strings.TrimSpace(ref)] = strings.TrimSpace(sha)
		}
	}
	return refs
}

func (r *repoClient) ListTags(ctx context.Context) ([]core.GitTag, error) {
	refs, err := r.fetchRefs(ctx)
	if err != nil {
		return nil, err
	}
	byTag := map[string]string{}
	for ref, sha := range refs {
		if !strings.HasPrefix(ref, "refs/tags/") {
			continue
		}
		name := strings.TrimPrefix(ref, "refs/tags/")
		if strings.HasSuffix(name, "^{}") {
			// Peeled annotated tag points directly to the commit SHA — prefer it over the tag object SHA.
			byTag[strings.TrimSuffix(name, "^{}")] = sha
		} else if _, exists := byTag[name]; !exists {
			byTag[name] = sha
		}
	}
	out := make([]core.GitTag, 0, len(byTag))
	for name, sha := range byTag {
		out = append(out, core.GitTag{Name: name, CommitSHA: sha})
	}
	return out, nil
}

func (r *repoClient) BranchHead(ctx context.Context, branch string) (core.GitCommit, error) {
	refs, err := r.fetchRefs(ctx)
	if err != nil {
		return core.GitCommit{}, err
	}
	key := "refs/heads/" + strings.TrimPrefix(branch, "refs/heads/")
	sha, ok := refs[key]
	if !ok {
		return core.GitCommit{}, fmt.Errorf("branch %q not found in remote refs", branch)
	}
	return core.GitCommit{SHA: sha}, nil
}

func (r *repoClient) ReadFile(ctx context.Context, ref, filePath string) ([]byte, error) {
	if r.rawFileURL == "" {
		return nil, errors.New("generic git requires raw_file_url template for git_file strategy (or use github/gitlab source)")
	}
	u := strings.ReplaceAll(r.rawFileURL, "{ref}", ref)
	u = strings.ReplaceAll(u, "{path}", strings.TrimPrefix(filePath, "/"))
	return core.DoRaw(ctx, core.DefaultClient, http.MethodGet, u, r.hdr)
}

func (Plugin) WebhookPlan(_ string, _ string, _ map[string]any) core.WebhookStatus {
	return core.WebhookStatus{
		Plugin:      "git",
		Kind:        "source",
		Supported:   false,
		Automatable: false,
		Note:        "Generic Git URLs are poll-only (checked every 5 minutes and on manual sync). Use github or gitlab for push/tag webhooks.",
	}
}

func (Plugin) ConnectWebhook(context.Context, map[string]any, map[string]any, string, string) (string, error) {
	return "", errors.New("generic git source does not support webhooks; use github or gitlab")
}
