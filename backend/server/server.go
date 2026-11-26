// Package server exposes a reusable bootstrap helper so both the open-source Community binary
// and the private Pro repository can wire plugins, sources, strategies, notifiers, and entitlements
// without duplicating server setup.
package server

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase"
	pbcore "github.com/pocketbase/pocketbase/core"

	"github.com/danysz/versionradar/backend/core"
	_ "github.com/danysz/versionradar/backend/pb_migrations"
	"github.com/danysz/versionradar/backend/plugins/providers/appstore"
	"github.com/danysz/versionradar/backend/plugins/providers/cloudflare"
	"github.com/danysz/versionradar/backend/plugins/providers/firebase"
	"github.com/danysz/versionradar/backend/plugins/providers/gcp"
	"github.com/danysz/versionradar/backend/plugins/providers/googleplay"
	"github.com/danysz/versionradar/backend/plugins/providers/heroku"
	"github.com/danysz/versionradar/backend/plugins/providers/githubactions"
	"github.com/danysz/versionradar/backend/plugins/providers/gitlabci"
	"github.com/danysz/versionradar/backend/plugins/providers/httpprobe"
	"github.com/danysz/versionradar/backend/plugins/providers/neon"
	"github.com/danysz/versionradar/backend/plugins/providers/netlify"
	"github.com/danysz/versionradar/backend/plugins/providers/render"
	"github.com/danysz/versionradar/backend/plugins/providers/supabase"
	"github.com/danysz/versionradar/backend/plugins/providers/vercel"
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
	CustomUIFS      fs.FS
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
		httpprobe.Plugin{},
		vercel.Plugin{},
		netlify.Plugin{},
		githubactions.Plugin{},
		gitlabci.Plugin{},
		render.Plugin{},
		supabase.Plugin{},
		neon.Plugin{},
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
	app := pocketbase.New()
	var publicURLFlag string
	app.RootCmd.PersistentFlags().StringVar(&publicURLFlag, "public-url", "", "Public URL for webhooks (e.g. https://xyz.trycloudflare.com)")

	encKey := os.Getenv("VR_ENCRYPTION_KEY")
	if encKey == "" {
		var err error
		encKey, err = resolveOrGenerateKey(app.DataDir(), app.Logger())
		if err != nil {
			return nil, nil, err
		}
	}
	cipher, err := core.NewCipher(encKey)
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
	app.OnServe().BindFunc(func(se *pbcore.ServeEvent) error {
		if publicURLFlag != "" {
			svc.PublicURL = strings.TrimRight(publicURLFlag, "/")
		}
		return se.Next()
	})
	svc.Bind(app)
	if err := RegisterUI(app, opts.CustomUIFS); err != nil {
		return nil, nil, err
	}
	return app, svc, nil
}

// resolveOrGenerateKey loads the persisted base64 encryption key from dataDir/encryption.key,
// or generates, persists (0600), and returns a new 32-byte key if none exists yet.
func resolveOrGenerateKey(dataDir string, logger *slog.Logger) (string, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", err
	}
	keyPath := filepath.Join(dataDir, "encryption.key")
	data, err := os.ReadFile(keyPath)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := base64.StdEncoding.EncodeToString(raw)
	if err := os.WriteFile(keyPath, []byte(key), 0600); err != nil {
		return "", err
	}
	if logger != nil {
		logger.Info("No VR_ENCRYPTION_KEY set: generated and persisted key to " + keyPath)
	}
	return key, nil
}
