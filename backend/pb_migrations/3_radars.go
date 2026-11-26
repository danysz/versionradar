package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// A "radar" is a product that also names a plugin and a target (what to watch at the provider).
func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("products")
		if err != nil {
			return err
		}
		c.Fields.Add(
			&core.SelectField{Name: "plugin_name", MaxSelect: 1, Values: []string{"cloudflare", "gcp", "googleplay", "appstore", "firebase"}},
			&core.TextField{Name: "target"}, // JSON object, non-secret (e.g. Pages account id + project)
		)
		return app.Save(c)
	}, nil)
}
