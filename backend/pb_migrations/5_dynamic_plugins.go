package migrations

import (
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Converts user_plugins.plugin_name and products.plugin_name from fixed SelectFields
// to TextFields (validated in Go against Registry / SourceRegistry / NotifierRegistry),
// preserving existing rows so adding new Community or Pro plugins needs zero DB migrations.
func init() {
	m.Register(func(app core.App) error {
		if err := swapSelectToText(app, "user_plugins", "plugin_name", true, "idx_user_plugins_user_plugin", "user, plugin_name"); err != nil {
			return err
		}
		return swapSelectToText(app, "products", "plugin_name", false, "", "")
	}, nil)
}

func swapSelectToText(app core.App, collection, field string, required bool, idxName, idxCols string) error {
	c, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return err
	}
	f := c.Fields.GetByName(field)
	if f == nil {
		return nil
	}
	if _, alreadyText := f.(*core.TextField); alreadyText {
		return nil
	}

	type row struct {
		ID  string `db:"id"`
		Val string `db:"val"`
	}
	var existing []row
	_ = app.DB().NewQuery("SELECT id, " + field + " AS val FROM " + collection).All(&existing)

	if idxName != "" {
		c.RemoveIndex(idxName)
	}
	c.Fields.RemoveByName(field)
	if err := app.Save(c); err != nil {
		return err
	}

	c, err = app.FindCollectionByNameOrId(collection)
	if err != nil {
		return err
	}
	c.Fields.Add(&core.TextField{
		Name:     field,
		Required: required,
	})
	if idxName != "" {
		c.AddIndex(idxName, true, idxCols, "")
	}
	if err := app.Save(c); err != nil {
		return err
	}

	for _, r := range existing {
		if r.Val == "" {
			continue
		}
		if _, err := app.DB().NewQuery("UPDATE "+collection+" SET "+field+" = {:v} WHERE id = {:id}").
			Bind(dbx.Params{"v": r.Val, "id": r.ID}).Execute(); err != nil {
			return err
		}
	}
	return nil
}
