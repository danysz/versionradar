package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		c, err := app.FindCollectionByNameOrId("user_plugins")
		if err != nil {
			return err
		}
		f := c.Fields.GetByName("plugin_name").(*core.SelectField)
		if !slices.Contains(f.Values, "firebase") {
			f.Values = append(f.Values, "firebase")
		}
		return app.Save(c)
	}, nil)
}
