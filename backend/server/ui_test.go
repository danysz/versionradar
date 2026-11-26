package server_test

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	pbui "github.com/pocketbase/pocketbase/ui"

	"github.com/danysz/versionradar/backend/core"
	_ "github.com/danysz/versionradar/backend/pb_migrations"
	"github.com/danysz/versionradar/backend/server"
	"github.com/danysz/versionradar/backend/strategies"
)

func newTestHandler(t *testing.T, customFS fs.FS) http.Handler {
	t.Helper()

	t.Setenv("VR_DEV_PASSWORD_AUTH", "1")
	t.Setenv("VR_GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("VR_GOOGLE_CLIENT_SECRET", "test-client-secret")

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(app.Cleanup)

	cipher, err := core.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	svc := &core.Service{
		App:        app,
		Cipher:     cipher,
		Registry:   core.NewRegistry(server.CommunityProviders()...),
		Sources:    core.NewSourceRegistry(server.CommunitySources()...),
		Strategies: core.NewStrategyRegistry(strategies.All()...),
		Notifiers:  core.NewNotifierRegistry(),
	}
	svc.Bind(app)

	if err := server.RegisterUI(app, customFS); err != nil {
		t.Fatalf("RegisterUI error: %v", err)
	}

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}
	if pbui.DistDirFS != nil {
		pbRouter.GET("/_/{path...}", apis.Static(pbui.DistDirFS, false))
	}

	serveEvent := &pbcore.ServeEvent{
		App:    app,
		Router: pbRouter,
	}

	var handler http.Handler
	if err := app.OnServe().Trigger(serveEvent, func(e *pbcore.ServeEvent) error {
		var buildErr error
		handler, buildErr = e.Router.BuildMux()
		return buildErr
	}); err != nil {
		t.Fatalf("OnServe failed: %v", err)
	}

	return handler
}

func doGET(t *testing.T, handler http.Handler, path string) (int, http.Header, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	return res.StatusCode, res.Header, rec.Body.String()
}

func TestEmbeddedUIServingAndRouting(t *testing.T) {
	handler := newTestHandler(t, nil)

	t.Run("GET / returns embedded index.html", func(t *testing.T) {
		status, _, body := doGET(t, handler, "/")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
		if !strings.Contains(body, "VersionRadar") && !strings.Contains(body, "<base href=\"/\">") {
			t.Fatalf("expected body to contain VersionRadar or <base href=\"/\">, got %q", body)
		}
	})

	t.Run("GET /radars falls back to index.html", func(t *testing.T) {
		status, _, body := doGET(t, handler, "/radars")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
		if !strings.Contains(body, "VersionRadar") && !strings.Contains(body, "<base href=\"/\">") {
			t.Fatalf("expected fallback index.html, got %q", body)
		}
	})

	t.Run("GET /api/v1/plugins preserves API route", func(t *testing.T) {
		status, hdr, body := doGET(t, handler, "/api/v1/plugins")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
		if ct := hdr.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("expected application/json Content-Type, got %q", ct)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("expected valid JSON, got error %v (body=%q)", err, body)
		}
		if payload["api_version"] != "v1" {
			t.Fatalf("expected api_version=v1, got %v", payload["api_version"])
		}
	})

	t.Run("GET /_/ preserves PocketBase Admin UI", func(t *testing.T) {
		if pbui.DistDirFS == nil {
			t.Skip("PocketBase ui.DistDirFS is nil")
		}
		status, _, body := doGET(t, handler, "/_/")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", status, body)
		}
		if strings.Contains(body, "VersionRadar UI Placeholder") {
			t.Fatalf("expected PocketBase Admin UI at /_/, got VersionRadar SPA fallback")
		}
	})
}

func TestCustomUIFSAndMissingIndex(t *testing.T) {
	t.Run("custom FS overrides embedded UI", func(t *testing.T) {
		custom := fstest.MapFS{
			"index.html": {Data: []byte("<html><body>Custom UI</body></html>")},
		}
		handler := newTestHandler(t, custom)

		status, _, body := doGET(t, handler, "/radars")
		if status != http.StatusOK {
			t.Fatalf("expected 200, got %d", status)
		}
		if !strings.Contains(body, "Custom UI") {
			t.Fatalf("expected custom UI body, got %q", body)
		}
	})

	t.Run("missing index.html skips UI registration without error", func(t *testing.T) {
		empty := fstest.MapFS{
			"other.txt": {Data: []byte("hello")},
		}
		handler := newTestHandler(t, empty)

		status, _, _ := doGET(t, handler, "/")
		if status != http.StatusNotFound {
			t.Fatalf("expected 404 when index.html missing, got %d", status)
		}

		apiStatus, _, _ := doGET(t, handler, "/api/v1/plugins")
		if apiStatus != http.StatusOK {
			t.Fatalf("expected API route to still return 200, got %d", apiStatus)
		}
	})

	t.Run("server.New accepts CustomUIFS option", func(t *testing.T) {
		t.Setenv("VR_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
		custom := fstest.MapFS{
			"index.html": {Data: []byte("<html><body>Custom</body></html>")},
		}
		app, svc, err := server.New(server.Options{CustomUIFS: custom})
		if err != nil {
			t.Fatalf("server.New failed: %v", err)
		}
		if app == nil || svc == nil {
			t.Fatal("expected non-nil app and svc")
		}
	})
}
