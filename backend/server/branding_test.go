package server_test

import (
	"bytes"
	"encoding/base64"
	"io/fs"
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
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if len(server.BrandFaviconPNG) == 0 || !bytes.HasPrefix(server.BrandFaviconPNG, pngMagic) {
		t.Fatalf("expected embedded BrandFaviconPNG to start with PNG magic bytes, got %d bytes", len(server.BrandFaviconPNG))
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

	for _, path := range []string{"/_/images/logo.svg", "/_/images/logo_white.svg", "/favicon.ico"} {
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

	for _, path := range []string{"/_/images/favicon.png", "/_/images/favicon_prod.png"} {
		status, hdr, body := doGET(t, handler, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d: %s", path, status, body)
		}
		if ct := hdr.Get("Content-Type"); !strings.Contains(ct, "image/png") {
			t.Fatalf("GET %s expected image/png Content-Type, got %q", path, ct)
		}
		if body != string(server.BrandFaviconPNG) {
			t.Fatalf("GET %s body did not match BrandFaviconPNG", path)
		}
	}

	t.Run("UIExtension script is served at /_/extensions.js", func(t *testing.T) {
		if pbui.DistDirFS == nil {
			t.Skip("PocketBase ui.DistDirFS is nil")
		}
		status, hdr, body := doGET(t, handler, "/_/extensions.js")
		if status != http.StatusOK {
			t.Fatalf("GET /_/extensions.js expected 200, got %d: %s", status, body)
		}
		if ct := hdr.Get("Content-Type"); !strings.Contains(ct, "text/javascript") {
			t.Fatalf("GET /_/extensions.js expected text/javascript Content-Type, got %q", ct)
		}
		if !strings.Contains(body, `window.app.store.favicon = "./images/favicon.png"`) {
			t.Fatalf("expected /_/extensions.js to set favicon, got %q", body)
		}
		if !strings.Contains(body, `mount:pageInstaller`) || !strings.Contains(body, `Setup your VersionRadar instance`) {
			t.Fatalf("expected /_/extensions.js to handle mount:pageInstaller title, got %q", body)
		}
	})

	t.Run("ui.DistDirFS serves index.html with VersionRadar title and favicon", func(t *testing.T) {
		if pbui.DistDirFS == nil {
			t.Skip("PocketBase ui.DistDirFS is nil")
		}
		rawIndex, err := fs.ReadFile(pbui.DistDirFS, "index.html")
		if err != nil {
			t.Fatalf("fs.ReadFile(ui.DistDirFS, index.html) failed: %v", err)
		}
		statInfo, err := fs.Stat(pbui.DistDirFS, "index.html")
		if err != nil {
			t.Fatalf("fs.Stat(ui.DistDirFS, index.html) failed: %v", err)
		}
		if statInfo.Size() != int64(len(rawIndex)) {
			t.Fatalf("expected Stat().Size()=%d to match read length %d", statInfo.Size(), len(rawIndex))
		}
		indexStr := string(rawIndex)
		if !strings.Contains(indexStr, "<title>VersionRadar</title>") {
			t.Fatalf("expected index.html to contain <title>VersionRadar</title>, got %q", indexStr)
		}
		if strings.Contains(indexStr, "<title>PocketBase</title>") {
			t.Fatalf("expected index.html not to contain <title>PocketBase</title>, got %q", indexStr)
		}
		if !strings.Contains(indexStr, `<link rel="icon" type="image/png" href="./images/favicon.png">`) {
			t.Fatalf("expected index.html to inject favicon link, got %q", indexStr)
		}

		status, _, body := doGET(t, handler, "/_/")
		if status != http.StatusOK {
			t.Fatalf("GET /_/ expected 200, got %d: %s", status, body)
		}
		if !strings.Contains(body, "<title>VersionRadar</title>") {
			t.Fatalf("expected GET /_/ response to contain <title>VersionRadar</title>, got %q", body)
		}
	})

	t.Run("ui.DistDirFS serves installer script without PocketBase instance", func(t *testing.T) {
		if pbui.DistDirFS == nil {
			t.Skip("PocketBase ui.DistDirFS is nil")
		}
		matches, err := fs.Glob(pbui.DistDirFS, "assets/pageInstaller-*.js")
		if err != nil {
			t.Fatalf("fs.Glob failed: %v", err)
		}
		if len(matches) == 0 {
			t.Fatal("expected at least one assets/pageInstaller-*.js file in ui.DistDirFS")
		}
		for _, match := range matches {
			rawJS, err := fs.ReadFile(pbui.DistDirFS, match)
			if err != nil {
				t.Fatalf("fs.ReadFile(%q) failed: %v", match, err)
			}
			statInfo, err := fs.Stat(pbui.DistDirFS, match)
			if err != nil {
				t.Fatalf("fs.Stat(%q) failed: %v", match, err)
			}
			if statInfo.Size() != int64(len(rawJS)) {
				t.Fatalf("expected Stat(%q).Size()=%d to match read length %d", match, statInfo.Size(), len(rawJS))
			}
			jsStr := string(rawJS)
			if strings.Contains(jsStr, "PocketBase instance") {
				t.Fatalf("expected %s not to contain 'PocketBase instance'", match)
			}
			if !strings.Contains(jsStr, "Setup your VersionRadar instance") {
				t.Fatalf("expected %s to contain 'Setup your VersionRadar instance'", match)
			}

			status, _, body := doGET(t, handler, "/_/"+match)
			if status != http.StatusOK {
				t.Fatalf("GET /_/%s expected 200, got %d", match, status)
			}
			if strings.Contains(body, "PocketBase instance") || !strings.Contains(body, "Setup your VersionRadar instance") {
				t.Fatalf("GET /_/%s returned unbranded installer script", match)
			}
		}
	})
}
