package core

import (
	"context"
	"net/http"
	"sort"
	"time"
)

// GitTag represents a tag in a Git repository.
type GitTag struct {
	Name      string
	CommitSHA string
}

// GitCommit represents a commit in a Git repository.
type GitCommit struct {
	SHA     string
	Message string
	Date    time.Time
}

// RepoClient is the host-neutral interface a VersionStrategy uses to inspect a repository.
// Implementing SourcePlugin + RepoClient lets any Git host work with every VersionStrategy plugin.
type RepoClient interface {
	ListTags(ctx context.Context) ([]GitTag, error)
	BranchHead(ctx context.Context, branch string) (GitCommit, error)
	ReadFile(ctx context.Context, ref, filePath string) ([]byte, error)
}

// SourcePlugin integrates a Git hosting provider (e.g. github, gitlab, generic git)
// so VersionRadar can fetch the latest version from code and optionally receive push/tag webhooks.
type SourcePlugin interface {
	Name() string
	Version() string
	ConfigFields() []Field
	TargetFields() []Field
	OpenRepo(ctx context.Context, cfg, target map[string]any) (RepoClient, error)
	VerifyWebhook(r *http.Request, body []byte, secret string) error
	ParsePushWebhook(body []byte) (ref, sha string, err error)
}

// SourceStub provides default no-op webhook methods for poll-only SourcePlugins.
type SourceStub struct{}

func (SourceStub) VerifyWebhook(*http.Request, []byte, string) error { return ErrNotImplemented }
func (SourceStub) ParsePushWebhook([]byte) (string, string, error)   { return "", "", ErrNotImplemented }

// VersionInfo is the resolved latest code version returned by a VersionStrategy plugin.
type VersionInfo struct {
	Version   string `json:"version"`
	CommitSHA string `json:"commit_sha,omitempty"`
	Ref       string `json:"ref,omitempty"`
}

// VersionStrategy is a pluggable strategy for determining the latest version from a source repository
// (e.g. git_tag, git_branch_commit, git_file). New strategies can be registered in StrategyRegistry
// without changing the client UI or database schema.
type VersionStrategy interface {
	Name() string
	Label() string
	Description() string
	ConfigFields() []Field
	Resolve(ctx context.Context, repo RepoClient, cfg map[string]any) (VersionInfo, error)
}

type SourceRegistry struct{ sources map[string]SourcePlugin }

func NewSourceRegistry(ps ...SourcePlugin) *SourceRegistry {
	r := &SourceRegistry{sources: map[string]SourcePlugin{}}
	for _, p := range ps {
		r.sources[p.Name()] = p
	}
	return r
}

func (r *SourceRegistry) Get(name string) (SourcePlugin, bool) {
	if r == nil {
		return nil, false
	}
	p, ok := r.sources[name]
	return p, ok
}

func (r *SourceRegistry) List() []SourcePlugin {
	if r == nil {
		return nil
	}
	out := make([]SourcePlugin, 0, len(r.sources))
	for _, p := range r.sources {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

type StrategyRegistry struct{ strategies map[string]VersionStrategy }

func NewStrategyRegistry(ss ...VersionStrategy) *StrategyRegistry {
	r := &StrategyRegistry{strategies: map[string]VersionStrategy{}}
	for _, s := range ss {
		r.strategies[s.Name()] = s
	}
	return r
}

func (r *StrategyRegistry) Get(name string) (VersionStrategy, bool) {
	if r == nil {
		return nil, false
	}
	s, ok := r.strategies[name]
	return s, ok
}

func (r *StrategyRegistry) List() []VersionStrategy {
	if r == nil {
		return nil
	}
	out := make([]VersionStrategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
