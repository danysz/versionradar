package server

import (
	"embed"
	"io/fs"

	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
)

//go:embed all:dist
var embeddedUI embed.FS

// RegisterUI serves the embedded (or custom) SPA assets from the root router while preserving
// more specific PocketBase API (/api/...) and Admin UI (/_/) routes.
func RegisterUI(app pbcore.App, customFS fs.FS) error {
	subFS := customFS
	if subFS == nil {
		var err error
		subFS, err = fs.Sub(embeddedUI, "dist")
		if err != nil {
			return err
		}
	}

	if _, err := fs.Stat(subFS, "index.html"); err != nil {
		app.Logger().Warn("UI index.html not found, skipping static UI registration", "error", err)
		return nil
	}

	app.OnServe().BindFunc(func(se *pbcore.ServeEvent) error {
		se.Router.GET("/{path...}", apis.Static(subFS, true))
		return se.Next()
	})

	return nil
}
