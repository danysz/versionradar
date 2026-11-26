package server

import (
	_ "embed"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

//go:embed logo.svg
var BrandLogoSVG []byte

func registerBranding(app core.App) {
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		settings := app.Settings()
		if settings.Meta.AppName == "" || settings.Meta.AppName == "Acme" || settings.Meta.AppName == "PocketBase" {
			settings.Meta.AppName = "VersionRadar"
			settings.Meta.SenderName = "VersionRadar"
			_ = app.Save(settings)
		}
		return nil
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/_/images/logo.svg", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/svg+xml", BrandLogoSVG)
		})
		e.Router.GET("/favicon.ico", func(re *core.RequestEvent) error {
			return re.Blob(http.StatusOK, "image/svg+xml", BrandLogoSVG)
		})
		return e.Next()
	})
}
