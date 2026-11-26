package migrations

import (
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Adds Heroku + Git source plugins, pluggable version-checking fields on radars (products),
// webhook automation status tracking, and build progress/stage/commit_sha on deployments.
func init() {
	m.Register(func(app core.App) error {
		up, err := app.FindCollectionByNameOrId("user_plugins")
		if err != nil {
			return err
		}
		if f, ok := up.Fields.GetByName("plugin_name").(*core.SelectField); ok {
			for _, v := range []string{"heroku", "github", "gitlab", "git"} {
				if !slices.Contains(f.Values, v) {
					f.Values = append(f.Values, v)
				}
			}
		}
		if err := app.Save(up); err != nil {
			return err
		}

		products, err := app.FindCollectionByNameOrId("products")
		if err != nil {
			return err
		}
		if f, ok := products.Fields.GetByName("plugin_name").(*core.SelectField); ok {
			if !slices.Contains(f.Values, "heroku") {
				f.Values = append(f.Values, "heroku")
			}
		}
		products.Fields.Add(
			// Git / code source for checking the latest expected version
			&core.TextField{Name: "source_plugin"},   // e.g. "github", "gitlab", "git" (validated against SourceRegistry)
			&core.TextField{Name: "source_target"},   // JSON object (e.g. {"repo":"owner/repo"})
			&core.TextField{Name: "version_strategy"}, // e.g. "git_tag", "git_branch_commit", "git_file" (validated against StrategyRegistry)
			&core.TextField{Name: "version_config"},  // JSON object for strategy options
			// Server-maintained latest code version snapshot (pushed via realtime on products)
			&core.TextField{Name: "latest_version"},
			&core.TextField{Name: "latest_commit"},
			&core.TextField{Name: "latest_ref"},
			&core.DateField{Name: "latest_checked_at"},
			&core.TextField{Name: "source_error"},
			// Webhook automation connection state
			&core.BoolField{Name: "webhook_connected"},
			&core.TextField{Name: "webhook_external_id"},
			&core.BoolField{Name: "source_webhook_connected"},
			&core.TextField{Name: "source_webhook_external_id"},
		)
		if err := app.Save(products); err != nil {
			return err
		}

		dep, err := app.FindCollectionByNameOrId("deployments")
		if err != nil {
			return err
		}
		dep.Fields.Add(
			&core.TextField{Name: "stage"},
			&core.NumberField{Name: "progress", Min: types.Pointer(0.0), Max: types.Pointer(100.0)},
			&core.TextField{Name: "commit_sha"},
		)
		return app.Save(dep)
	}, nil)
}
