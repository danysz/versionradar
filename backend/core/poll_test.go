package core_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/danysz/versionradar/backend/core"
	_ "github.com/danysz/versionradar/backend/pb_migrations"
	"github.com/danysz/versionradar/backend/strategies"
)

type fakePlugin struct {
	core.Stub
	got []map[string]any
}

func (*fakePlugin) Name() string    { return "cloudflare" }
func (*fakePlugin) Version() string { return "9.9.9" }
func (f *fakePlugin) CheckStatus(_ context.Context, cfg map[string]any) ([]core.DeploymentEvent, error) {
	f.got = core.Targets(cfg)
	var out []core.DeploymentEvent
	for _, t := range f.got {
		out = append(out, core.DeploymentEvent{
			ProductID:  core.Str(t, "product"),
			ExternalID: "d-" + core.Str(t, "project"),
			Platform:   "cloudflare",
			Status:     core.StatusSuccess,
			Version:    core.Str(t, "project"),
			Stage:      "deploy",
			Progress:   100,
		})
	}
	out = append(out, core.DeploymentEvent{ProductID: "someone-elses", ExternalID: "x", Platform: "cloudflare", Status: core.StatusFailed})
	return out, nil
}

type fakeRepo struct{}

func (fakeRepo) ListTags(context.Context) ([]core.GitTag, error) {
	return []core.GitTag{
		{Name: "v1.2.0", CommitSHA: "1111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{Name: "v1.10.0", CommitSHA: "abcdef1234567890abcdef1234567890abcdef12"},
		{Name: "v1.9.5", CommitSHA: "2222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}, nil
}
func (fakeRepo) BranchHead(context.Context, string) (core.GitCommit, error) {
	return core.GitCommit{SHA: "abcdef1234567890abcdef1234567890abcdef12"}, nil
}
func (fakeRepo) ReadFile(context.Context, string, string) ([]byte, error) {
	return []byte("name: app\nversion: 2.3.4+5\n"), nil
}

type fakeSource struct{ core.SourceStub }

func (fakeSource) Name() string          { return "github" }
func (fakeSource) Version() string       { return "1.0.0" }
func (fakeSource) ConfigFields() []core.Field { return nil }
func (fakeSource) TargetFields() []core.Field { return nil }
func (fakeSource) OpenRepo(context.Context, map[string]any, map[string]any) (core.RepoClient, error) {
	return fakeRepo{}, nil
}

// Each of a user's radars for a plugin becomes one target; events only land in the user's own radars,
// and Git version sources resolve latest_version / latest_commit onto the radar record.
func TestSyncUserBuildsTargetsFromRadarsAndSyncsGitSource(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()
	cipher, _ := core.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	fp := &fakePlugin{}
	svc := &core.Service{
		App:        app,
		Registry:   core.NewRegistry(fp),
		Sources:    core.NewSourceRegistry(fakeSource{}),
		Strategies: core.NewStrategyRegistry(strategies.All()...),
		Cipher:     cipher,
	}
	svc.Bind(app)

	users, _ := app.FindCollectionByNameOrId("users")
	u := pbcore.NewRecord(users)
	u.SetEmail("a@b.co")
	u.SetPassword("password123")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	products, _ := app.FindCollectionByNameOrId("products")
	var firstRadarID string
	for i, p := range []string{"site", "app"} {
		r := pbcore.NewRecord(products)
		r.Set("user", u.Id)
		r.Set("name", p)
		r.Set("plugin_name", "cloudflare")
		r.Set("target", `{"account_id":"a","project":"`+p+`"}`)
		if i == 0 {
			r.Set("source_plugin", "github")
			r.Set("source_target", `{"repo":"org/site"}`)
			r.Set("version_strategy", "git_tag")
			r.Set("version_config", `{"tag_prefix":"v"}`)
		}
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstRadarID = r.Id
		}
	}
	ups, _ := app.FindCollectionByNameOrId("user_plugins")
	up := pbcore.NewRecord(ups)
	up.Set("user", u.Id)
	up.Set("plugin_name", "cloudflare")
	up.Set("is_enabled", true)
	up.Set("config_json", `{"api_token":"t"}`)
	if err := app.Save(up); err != nil {
		t.Fatal(err)
	}

	n, srcN, _, err := svc.SyncUser(u.Id)
	if err != nil || n != 2 || srcN != 1 || len(fp.got) != 2 {
		t.Fatalf("events=%d sources=%d targets=%d err=%v", n, srcN, len(fp.got), err)
	}
	deps, _ := app.FindAllRecords("deployments")
	if len(deps) != 2 {
		t.Fatalf("deployments=%d", len(deps))
	}
	if deps[0].GetInt("progress") != 100 || deps[0].GetString("stage") != "deploy" {
		t.Fatalf("stage/progress not saved: %+v", deps[0])
	}
	// Git source resolved highest semver tag v1.10.0 (beating v1.9.5)
	radar, err := app.FindRecordById("products", firstRadarID)
	if err != nil {
		t.Fatal(err)
	}
	if radar.GetString("latest_version") != "v1.10.0" || !strings.HasPrefix(radar.GetString("latest_commit"), "abcdef1") {
		t.Fatalf("unexpected git latest: ver=%q commit=%q err=%q",
			radar.GetString("latest_version"), radar.GetString("latest_commit"), radar.GetString("source_error"))
	}

	// second sync updates the same rows (ExternalID dedupe)
	if _, _, _, err := svc.SyncUser(u.Id); err != nil {
		t.Fatal(err)
	}
	if deps, _ = app.FindAllRecords("deployments"); len(deps) != 2 {
		t.Fatalf("after resync deployments=%d", len(deps))
	}
}
