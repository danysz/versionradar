# Connecting your accounts (read this before pasting any key)

VersionRadar stores all credentials **AES-256-GCM encrypted** and never returns them to the client. You can revoke any token at the provider at any time.

In the app:
1. Open **Plugins & Sources** to add credentials for your Deployment Providers and Git Sources.
2. Tap **Add Radar** (or edit an existing radar) to pick the deployment provider target and optionally attach a **Git Source** + **Version Check Strategy** (`git_tag`, `git_branch_commit`, or `git_file`).
3. Open **Webhook Automation** (bolt icon) to connect webhooks in one click — or copy the generated **Fallback CLI Script** if your saved credentials are strictly read-only.

## Deployment Providers

| Provider | Create | Minimum permission (poll) | Auto-connect webhook permission (optional) |
|---|---|---|---|
| **Cloudflare Pages** | API token | Account > Cloudflare Pages > **Read** | Add Account > Notifications > **Edit** (or use one-time token / CLI fallback) |
| **Heroku** | API key / OAuth token | Heroku Account API Key | Included (subscribes to `api:build` & `api:release`) |
| **GCP Cloud Build** | Service account + JSON key | **Cloud Build Viewer** (`roles/cloudbuild.builds.viewer`) | Add **Pub/Sub Editor** (`roles/pubsub.editor`) for auto push subscription (or run `gcloud` fallback) |
| **App Store Connect** | API key (`.p8`) | **Developer** (lowest role that reads app versions) | **Admin** role for Webhooks API (or configure manually in App Store Connect) |
| **Firebase App Distribution** | Service account + JSON key | **Firebase App Distribution Viewer** (`roles/firebaseappdistro.viewer`) | Poll only (provider has no release webhooks) |
| **Google Play** | Service account invited in Play Console | **View app information (read-only)** | Poll only (provider has no release webhooks) |

## Git Version Sources

| Source | Create | Minimum permission | Webhook (instant latest-version check) |
|---|---|---|---|
| **GitHub** | Fine-grained PAT (optional for public repos) | Repository > Contents > **Read** | Add Webhooks > **Write** for 1-click connect (or run `gh api` fallback) |
| **GitLab** | Personal/Project Access Token (optional for public repos) | `read_api` / `read_repository` | `api` scope for 1-click connect (or run `curl` fallback) |
| **Generic Git (HTTPS)** | Optional username + read-only token | Read access over HTTPS Smart HTTP (`info/refs`) | Poll only (every 5 minutes + manual sync) |

## Webhook URL format
`https://api.versionradar.app/api/v1/webhooks/<your user id>/<plugin>?product=<radar id>`

Pull runs every 5 minutes and whenever you tap **Sync**.
