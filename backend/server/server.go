// Package server exposes a reusable bootstrap helper so both the open-source Community binary
// and the private Pro repository can wire plugins, sources, strategies, notifiers, and entitlements
// without duplicating server setup.
package server

import (
	"os"

	"github.com/pocketbase/pocketbase"

	"github.com/danysz/versionradar/backend/core"
	_ "github.com/danysz/versionradar/backend/pb_migrations"
	"github.com/danysz/versionradar/backend/plugins/providers/appstore"
	"github.com/danysz/versionradar/backend/plugins/providers/cloudflare"
	"github.com/danysz/versionradar/backend/plugins/providers/firebase"
	"github.com/danysz/versionradar/backend/plugins/providers/gcp"
	"github.com/danysz/versionradar/backend/plugins/providers/googleplay"
	"github.com/danysz/versionradar/backend/plugins/providers/heroku"
	"github.com/danysz/versionradar/backend/plugins/providers/githubactions"
	"github.com/danysz/versionradar/backend/sources/git"
	"github.com/danysz/versionradar/backend/sources/github"
	"github.com/danysz/versionradar/backend/sources/gitlab"
	"github.com/danysz/versionradar/backend/strategies"
)

// Options lets an edition (Community or Pro) register extra plugins and custom entitlements.
type Options struct {
	ExtraProviders  []core.ProviderPlugin
	ExtraSources    []core.SourcePlugin
	ExtraStrategies []core.VersionStrategy
	ExtraNotifiers  []core.NotifierPlugin
	Entitlements    core.EntitlementChecker
	PublicURL       string
}

// CommunityProviders returns the built-in open-source deployment provider plugins.
func CommunityProviders() []core.ProviderPlugin {
	return []core.ProviderPlugin{
		cloudflare.Plugin{},
		firebase.Plugin{},
		gcp.Plugin{},
		googleplay.Plugin{},
		appstore.Plugin{},
		heroku.Plugin{},
		githubactions.Plugin{},
	}
}

// CommunitySources returns the built-in open-source Git source plugins.
func CommunitySources() []core.SourcePlugin {
	return []core.SourcePlugin{
		github.Plugin{},
		gitlab.Plugin{},
		git.Plugin{},
	}
}

// New constructs a configured PocketBase app + core.Service ready to call app.Start().
func New(opts Options) (*pocketbase.PocketBase, *core.Service, error) {
	cipher, err := core.NewCipher(os.Getenv("VR_ENCRYPTION_KEY"))
	if err != nil {
		return nil, nil, err
	}
	providers := append(CommunityProviders(), opts.ExtraProviders...)
	sources := append(CommunitySources(), opts.ExtraSources...)
	strats := append(strategies.All(), opts.ExtraStrategies...)
	notifiers := opts.ExtraNotifiers

	publicURL := opts.PublicURL
	if publicURL == "" {
		publicURL = os.Getenv("VR_PUBLIC_URL")
	}

	app := pocketbase.New()
	svc := &core.Service{
		App:          app,
		Cipher:       cipher,
		Registry:     core.NewRegistry(providers...),
		Sources:      core.NewSourceRegistry(sources...),
		Strategies:   core.NewStrategyRegistry(strats...),
		Notifiers:    core.NewNotifierRegistry(notifiers...),
		Entitlements: opts.Entitlements,
		PublicURL:    publicURL,
	}
	svc.Bind(app)
	return app, svc, nil
}
