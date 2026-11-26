// Package strategies provides built-in pluggable VersionStrategy implementations.
// Anyone can add a new way to read a version from code by implementing core.VersionStrategy
// and registering it in main.go's StrategyRegistry.
package strategies

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/danysz/versionradar/backend/core"
)

// GitTag resolves the latest Git tag (semver-aware), optionally filtered by a prefix (e.g. "v" or "app-v").
type GitTag struct{}

func (GitTag) Name() string        { return "git_tag" }
func (GitTag) Label() string       { return "Git Tag (latest)" }
func (GitTag) Description() string { return "Finds the highest semver/latest tag in the repository" }

func (GitTag) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "tag_prefix", Label: "Tag prefix filter (optional)", Help: "e.g. v or web-v (leave empty for all tags)", Optional: true},
	}
}

func (GitTag) Resolve(ctx context.Context, repo core.RepoClient, cfg map[string]any) (core.VersionInfo, error) {
	tags, err := repo.ListTags(ctx)
	if err != nil {
		return core.VersionInfo{}, err
	}
	prefix := strings.TrimSpace(core.Str(cfg, "tag_prefix"))
	var matched []core.GitTag
	for _, t := range tags {
		if prefix == "" || strings.HasPrefix(t.Name, prefix) {
			matched = append(matched, t)
		}
	}
	if len(matched) == 0 {
		return core.VersionInfo{}, errors.New("no matching git tags found")
	}
	sort.Slice(matched, func(i, j int) bool {
		a := strings.TrimPrefix(matched[i].Name, prefix)
		b := strings.TrimPrefix(matched[j].Name, prefix)
		return core.CompareSemver(a, b) > 0
	})
	best := matched[0]
	ver := best.Name
	if prefix != "" && prefix != "v" && prefix != "V" {
		ver = strings.TrimPrefix(best.Name, prefix)
	}
	return core.VersionInfo{
		Version:   ver,
		CommitSHA: best.CommitSHA,
		Ref:       "tag " + best.Name,
	}, nil
}

// GitBranchCommit resolves the latest commit SHA on a named branch (e.g. main).
type GitBranchCommit struct{}

func (GitBranchCommit) Name() string        { return "git_branch_commit" }
func (GitBranchCommit) Label() string       { return "Branch Latest Commit" }
func (GitBranchCommit) Description() string { return "Tracks the HEAD commit SHA of a specific branch" }

func (GitBranchCommit) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "branch", Label: "Branch name", Help: "e.g. main or production", Default: "main"},
	}
}

func (GitBranchCommit) Resolve(ctx context.Context, repo core.RepoClient, cfg map[string]any) (core.VersionInfo, error) {
	branch := strings.TrimSpace(core.Str(cfg, "branch"))
	if branch == "" {
		branch = "main"
	}
	commit, err := repo.BranchHead(ctx, branch)
	if err != nil {
		return core.VersionInfo{}, err
	}
	return core.VersionInfo{
		Version:   core.ShortSHA(commit.SHA, ""),
		CommitSHA: commit.SHA,
		Ref:       "branch " + branch,
	}, nil
}

// GitFile reads a version from a file in the repository (pubspec.yaml, package.json, Cargo.toml, VERSION, or custom regex).
type GitFile struct{}

func (GitFile) Name() string  { return "git_file" }
func (GitFile) Label() string { return "Version File in Code" }
func (GitFile) Description() string {
	return "Reads the version from a file (package.json, pubspec.yaml, Cargo.toml, VERSION, or custom regex)"
}

func (GitFile) ConfigFields() []core.Field {
	return []core.Field{
		{Key: "branch", Label: "Branch or ref", Help: "e.g. main", Default: "main"},
		{Key: "file_path", Label: "File path in repo", Help: "e.g. package.json, pubspec.yaml, VERSION, Cargo.toml"},
		{Key: "regex", Label: "Custom regex (optional)", Help: `Optional regex with capture group, e.g. versionName\s+"([^"]+)"`, Optional: true},
	}
}

func (GitFile) Resolve(ctx context.Context, repo core.RepoClient, cfg map[string]any) (core.VersionInfo, error) {
	branch := strings.TrimSpace(core.Str(cfg, "branch"))
	if branch == "" {
		branch = "main"
	}
	filePath := strings.TrimSpace(core.Str(cfg, "file_path"))
	if filePath == "" {
		return core.VersionInfo{}, errors.New("file_path is required")
	}
	raw, err := repo.ReadFile(ctx, branch, filePath)
	if err != nil {
		return core.VersionInfo{}, err
	}
	ver, err := core.ExtractVersionFromFile(filePath, raw, core.Str(cfg, "regex"))
	if err != nil {
		return core.VersionInfo{}, err
	}
	commitSHA := ""
	if head, err := repo.BranchHead(ctx, branch); err == nil {
		commitSHA = head.SHA
	}
	return core.VersionInfo{
		Version:   ver,
		CommitSHA: commitSHA,
		Ref:       fmt.Sprintf("%s@%s", filePath, branch),
	}, nil
}

// All returns all built-in VersionStrategy plugins.
func All() []core.VersionStrategy {
	return []core.VersionStrategy{GitTag{}, GitBranchCommit{}, GitFile{}}
}
