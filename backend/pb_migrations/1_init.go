package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Rules: owners only. Deployments are read-only for clients (the server writes them),
// and readable rules are what make realtime subscriptions work.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		own := types.Pointer("user = @request.auth.id")
		create := types.Pointer(`@request.auth.id != "" && @request.body.user = @request.auth.id`)
		update := types.Pointer("user = @request.auth.id && @request.body.user:isset = false")

		products := core.NewBaseCollection("products")
		products.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "name", Required: true},
			&core.TextField{Name: "description"},
		)
		products.ListRule, products.ViewRule, products.DeleteRule = own, own, own
		products.CreateRule, products.UpdateRule = create, update
		if err := app.Save(products); err != nil {
			return err
		}

		up := core.NewBaseCollection("user_plugins")
		up.Fields.Add(
			&core.RelationField{Name: "user", CollectionId: users.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{Name: "plugin_name", Required: true, MaxSelect: 1, Values: []string{"cloudflare", "gcp", "googleplay", "appstore"}},
			&core.BoolField{Name: "is_enabled"},
			&core.TextField{Name: "config_json", Hidden: true}, // AES-GCM ciphertext, never returned to clients
			&core.TextField{Name: "webhook_secret"},            // server-generated; owner reads it to configure the provider
			&core.TextField{Name: "plugin_version"},            // server-set: plugin version at install time
		)
		up.AddIndex("idx_user_plugins_user_plugin", true, "user, plugin_name", "")
		locked := " && @request.body.webhook_secret:isset = false && @request.body.plugin_version:isset = false"
		up.ListRule, up.ViewRule, up.DeleteRule = own, own, own
		up.CreateRule = types.Pointer(*create + locked)
		up.UpdateRule = types.Pointer(*update + locked)
		if err := app.Save(up); err != nil {
			return err
		}

		dep := core.NewBaseCollection("deployments")
		dep.Fields.Add(
			&core.RelationField{Name: "product", CollectionId: products.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "platform"},
			&core.TextField{Name: "external_id"}, // provider-side id, dedupes webhooks and polls
			&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"success", "failed", "in_progress"}},
			&core.TextField{Name: "version"},
			&core.TextField{Name: "message"},
			&core.DateField{Name: "timestamp"},
		)
		dep.AddIndex("idx_deployments_product_ts", false, "product, timestamp", "")
		dep.AddIndex("idx_deployments_external", true, "product, platform, external_id", "external_id != ''")
		read := types.Pointer("product.user = @request.auth.id")
		dep.ListRule, dep.ViewRule = read, read // create/update/delete stay nil = superuser/server only
		return app.Save(dep)
	}, func(app core.App) error {
		for _, n := range []string{"deployments", "user_plugins", "products"} {
			if c, err := app.FindCollectionByNameOrId(n); err == nil {
				if err := app.Delete(c); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
