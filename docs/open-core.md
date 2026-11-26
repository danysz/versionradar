# Open-Core Architecture: Community (GitHub) vs. Pro (Private GitLab)

VersionRadar uses a clean **two-repository Open-Core** model with **zero code duplication** and **one shared Flutter client**:

1. **Community (Public on GitHub — `github.com/danysz/versionradar`, Apache-2.0)**
   - Contains `backend/core`, `backend/server`, `backend/pb_migrations`, Community plugins/sources/strategies, and `client/`.
   - Exposes `github.com/danysz/versionradar/backend/server` so Pro imports it as a standard Go module.
   - Because `products.plugin_name` and `user_plugins.plugin_name` are dynamic text fields, adding new Community or Pro plugins requires **zero DB migrations**.

2. **Pro (Private on GitLab — `gitlab.com/dc-ds/versionradar/versionradar-core`)**
   - Imports `github.com/danysz/versionradar/backend` in `backend/go.mod`.
   - Adds Pro-only plugins (Cloud Run, TestFlight, Slack/Discord/Telegram alerts, etc.), license/plan entitlements (`VR_LICENSE_KEY` Ed25519 offline keys or SaaS mode), and production Cloud Run / Litestream / Cloudflare Pages deployment tooling.

## Example `versionradar-pro/backend/cmd/server/main.go`

```go
package main

import (
    "log"

    "github.com/danysz/versionradar/backend/core"
    "github.com/danysz/versionradar/backend/server"
    "gitlab.com/dc-ds/versionradar/versionradar-core/backend/entitlements"
)

func main() {
    ent := entitlements.NewFromEnv()
    app, _, err := server.New(server.Options{
        Entitlements: ent,
        ExtraProviders: []core.ProviderPlugin{
            // Register Pro provider plugins here
        },
        ExtraNotifiers: []core.NotifierPlugin{
            // Register Pro notifier plugins here
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```
