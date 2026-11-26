package server_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
	pbui "github.com/pocketbase/pocketbase/ui"

	"github.com/danysz/versionradar/backend/server"
)

func TestBrandingAndNoOpenFlag(t *testing.T) {
	if len(server.BrandLogoSVG) == 0 || !bytes.Contains(server.BrandLogoSVG, []byte("<svg")) {
		t.Fatalf("expected embedded BrandLogoSVG to contain SVG markup, got %d bytes", len(server.BrandLogoSVG))
	}

	t.Setenv("VR_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))

	dataDir := t.TempDir()
	origArgs := os.Args
	os.Args = []string{"versionradar", "--dir=" + dataDir}
	t.Cleanup(func() { os.Args = origArgs })

	app, _, err := server.New(server.Options{})
	if err != nil {
		t.Fatalf("server.New failed: %v", err)
	}
	if err := app.RootCmd.PersistentFlags().Set("dir", dataDir); err != nil {
		t.Fatalf("failed to set --dir flag: %v", err)
	}

	noOpenFlag := app.RootCmd.PersistentFlags().Lookup("no-open")
	if noOpenFlag == nil {
		t.Fatal("expected --no-open persistent flag to be registered on app.RootCmd")
	}
	if noOpenFlag.DefValue != "false" {
		t.Fatalf("expected --no-open default to be false, got %q", noOpenFlag.DefValue)
	}

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("app.Bootstrap failed: %v", err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })

	if app.Settings().Meta.AppName != "VersionRadar" {
		t.Fatalf("expected Meta.AppName to be %q, got %q", "VersionRadar", app.Settings().Meta.AppName)
	}
	if app.Settings().Meta.SenderName != "VersionRadar" {
		t.Fatalf("expected Meta.SenderName to be %q, got %q", "VersionRadar", app.Settings().Meta.SenderName)
	}

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}
	if pbui.DistDirFS != nil {
		pbRouter.GET("/_/{path...}", apis.Static(pbui.DistDirFS, false))
	}

	var handler http.Handler
	if err := app.OnServe().Trigger(&pbcore.ServeEvent{App: app, Router: pbRouter}, func(e *pbcore.ServeEvent) error {
		var buildErr error
		handler, buildErr = e.Router.BuildMux()
		return buildErr
	}); err != nil {
		t.Fatalf("OnServe failed: %v", err)
	}

	for _, path := range []string{"/_/images/logo.svg", "/favicon.ico"} {
		status, hdr, body := doGET(t, handler, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d: %s", path, status, body)
		}
		if ct := hdr.Get("Content-Type"); !strings.Contains(ct, "image/svg+xml") {
			t.Fatalf("GET %s expected image/svg+xml Content-Type, got %q", path, ct)
		}
		if body != string(server.BrandLogoSVG) {
			t.Fatalf("GET %s body did not match BrandLogoSVG", path)
		}
	}
}
