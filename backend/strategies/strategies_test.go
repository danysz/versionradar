package strategies

import (
	"context"
	"testing"

	"github.com/danysz/versionradar/backend/core"
)

type mockRepo struct{}

func (mockRepo) ListTags(context.Context) ([]core.GitTag, error) {
	return []core.GitTag{
		{Name: "v1.2.0", CommitSHA: "aaa1111"},
		{Name: "v1.10.0", CommitSHA: "bbb2222"},
		{Name: "api-v2.0.0", CommitSHA: "ccc3333"},
	}, nil
}

func (mockRepo) BranchHead(_ context.Context, branch string) (core.GitCommit, error) {
	return core.GitCommit{SHA: "deadbeef1234567890"}, nil
}

func (mockRepo) ReadFile(_ context.Context, _, _ string) ([]byte, error) {
	return []byte(`{"name":"demo","version":"2.5.1"}`), nil
}

func TestStrategies(t *testing.T) {
	ctx := context.Background()
	repo := mockRepo{}

	info, err := (GitTag{}).Resolve(ctx, repo, map[string]any{"tag_prefix": "v"})
	if err != nil || info.Version != "v1.10.0" || info.CommitSHA != "bbb2222" {
		t.Fatalf("GitTag: %+v %v", info, err)
	}

	info, err = (GitTag{}).Resolve(ctx, repo, map[string]any{"tag_prefix": "api-v"})
	if err != nil || info.Version != "2.0.0" || info.CommitSHA != "ccc3333" {
		t.Fatalf("GitTag prefixed: %+v %v", info, err)
	}

	info, err = (GitBranchCommit{}).Resolve(ctx, repo, map[string]any{"branch": "main"})
	if err != nil || info.Version != "deadbee" || info.CommitSHA != "deadbeef1234567890" {
		t.Fatalf("GitBranchCommit: %+v %v", info, err)
	}

	info, err = (GitFile{}).Resolve(ctx, repo, map[string]any{"branch": "main", "file_path": "package.json"})
	if err != nil || info.Version != "2.5.1" || info.CommitSHA != "deadbeef1234567890" {
		t.Fatalf("GitFile: %+v %v", info, err)
	}
}
