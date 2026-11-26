# VersionRadar (Community Edition)

<p align="center">
  <img src="brand/logo.svg" width="96" height="96" alt="VersionRadar Logo" />
</p>

<h3 align="center">Every release & Git version, live on one radar.</h3>

<p align="center">
  <a href="https://github.com/danysz/versionradar/actions"><img src="https://img.shields.io/badge/CI-Passing-2DD4BF?style=flat-square&logo=githubactions&logoColor=white" alt="CI Status" /></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version" /></a>
  <a href="https://flutter.dev"><img src="https://img.shields.io/badge/Flutter-3.27+-02569B?style=flat-square&logo=flutter&logoColor=white" alt="Flutter Version" /></a>
  <a href="https://pocketbase.io"><img src="https://img.shields.io/badge/PocketBase-v0.25+-B8DBE4?style=flat-square&logo=sqlite&logoColor=black" alt="PocketBase" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache--2.0-F59E0B?style=flat-square" alt="License" /></a>
</p>

---

**VersionRadar** is an open-source, multi-tenant deployment radar and Git drift tracker built with **Go + PocketBase** and **Flutter**.

Connect your cloud providers, CI/CD runners, serverless databases, and Git repositories with read-only credentials (encrypted at rest with **AES-256-GCM**) to see live build progress, deployment status, and whether the code currently running in production matches your latest Git release.

---

## ✨ Features

- **Live Deployed vs. Git Latest Comparison**: Side-by-side verification of deployed releases against your Git repository to instantly catch deployment drift.
- **Real-Time Push Updates**: Server-Sent Events (SSE) push deployment state changes and build progress to all connected clients instantly.
- **14 Built-in Deployment Providers**:
  - **Frontend & Edge**: Vercel, Netlify, Cloudflare Pages, Render.com.
  - **CI/CD Pipelines**: GitHub Actions, GitLab CI (supports self-hosted instances).
  - **Databases & Backends**: Supabase (DB migrations & Edge Functions), Neon (Serverless Postgres branches & ops), Heroku.
  - **Cloud Infrastructure**: Google Cloud Build (GCP).
  - **Mobile App Stores**: App Store Connect (iOS releases & review state), Google Play Console (Android release tracks), Firebase App Distribution.
  - **Universal**: HTTP Endpoint Probe (`/version`, `/health` or custom JSON pointer + CI webhook).
- **3 Git Version Sources**:
  - **GitHub** (REST API with tags, branches, and raw file fetching).
  - **GitLab** (REST API with self-hosted custom `base_url`).
  - **Generic Git over HTTPS** (Pure-Go Smart HTTP `info/refs?service=git-upload-pack` — works with any Git server).
- **3 Pluggable Version Check Strategies**:
  - `git_tag`: Semantic versioning resolution (`v1.2.3`, `1.2.3`, or custom prefix filters).
  - `git_branch_commit`: Tracks the HEAD commit SHA of any branch (e.g., `main`, `staging`).
  - `git_file`: Extracts version strings from repository files (`package.json`, `pubspec.yaml`, `Cargo.toml`, `VERSION`, or custom regex).
- **1-Click Webhook Automation**: Automatically creates incoming webhooks across GitHub, GitLab, Vercel, Netlify, and Render via API, or provides instant copy-paste curl/CLI fallback commands.
- **Zero-Migration Dynamic Plugins**: Registering new providers and notifiers requires zero database migrations (PocketBase dynamic schema validation).
- **Secure by Default**: All sensitive API keys and personal access tokens are encrypted with AES-256-GCM before hitting SQLite.

---

## 🚀 Quick Start

### Option 1: Docker Compose (One-Command Setup)

```bash
# Clone the repository
git clone https://github.com/danysz/versionradar.git
cd versionradar

# Generate an encryption key and boot the stack
export VR_ENCRYPTION_KEY=$(openssl rand -base64 32)
docker compose up -d
```

Open `http://localhost:18899` in your browser. The embedded Flutter Web client will load immediately, pointing to the local PocketBase backend.

---

### Option 2: Local Development

#### 1. Backend (Go + PocketBase)
```bash
cd backend

# Required: AES-256-GCM encryption key
export VR_ENCRYPTION_KEY=$(openssl rand -base64 32)

# Optional for local testing without Google OAuth
export VR_DEV_PASSWORD_AUTH=1

# Optional superuser credentials
export VR_ADMIN_EMAIL=admin@example.com
export VR_ADMIN_PASSWORD=password123456

go run ./cmd/server serve
```
The REST API and PocketBase Admin UI will run at `http://127.0.0.1:18899`.

#### 2. Flutter Client (Web, macOS, iOS, Android)
```bash
cd client
flutter run -d chrome --dart-define=API_URL=http://127.0.0.1:18899
```

---

## 🛠️ Environment Configuration

| Variable | Description | Default / Example |
| :--- | :--- | :--- |
| `VR_ENCRYPTION_KEY` | **Required**. Base64-encoded 32-byte key used for AES-256-GCM encryption. | `openssl rand -base64 32` |
| `VR_PUBLIC_URL` | Public base URL used to construct incoming webhook listener URLs. | `https://api.yourdomain.com` |
| `VR_DEV_PASSWORD_AUTH` | Enables email + password authentication for local development. | `1` (enabled) or `0` |
| `VR_GOOGLE_CLIENT_ID` | Google OAuth2 Client ID for user sign-in. | `...apps.googleusercontent.com` |
| `VR_GOOGLE_CLIENT_SECRET` | Google OAuth2 Client Secret for user sign-in. | `GOCSPX-...` |
| `VR_ADMIN_EMAIL` | Auto-creates initial PocketBase superuser. | `admin@example.com` |
| `VR_ADMIN_PASSWORD` | Password for the initial PocketBase superuser. | `min 10 characters` |

---

## 🧩 Architecture

```
versionradar/
├── backend/
│   ├── cmd/server/             # Community server entrypoint
│   ├── core/                   # Service, cipher, plugin interfaces, router
│   ├── pb_migrations/          # Dynamic PocketBase migrations
│   ├── plugins/providers/      # 14 Deployment Provider plugins
│   ├── sources/                # Git hosts (github, gitlab, git smart http)
│   ├── strategies/             # Version detection strategies (git_tag, commit, file)
│   └── server/                 # Reusable server bootstrap wiring Community & Pro
├── client/                     # Shared Flutter client (Web, macOS, iOS, Android)
│   ├── lib/pages/              # Radars, Deployments, Webhooks, Plugins, Login
│   ├── lib/widgets/            # UI kit, dialogs, modals, health indicators
│   └── lib/theme.dart          # Dynamic provider metadata, colors, and iconography
├── docs/                       # Credential guides and open-core design
├── docker-compose.yml          # Production self-hosting configuration
└── Dockerfile                  # Standalone multi-stage build container
```

### Pluggable Interfaces (`backend/core`)

1. **`ProviderPlugin`** (`core/plugin.go`): Handles polling provider APIs, parsing incoming webhooks, and optional `WebhookAutomator` 1-click connect logic.
2. **`SourcePlugin`** (`core/source.go`): Handles Git host authentication, branch commit discovery, file fetching, and push/tag webhooks.
3. **`VersionStrategy`** (`core/source.go`): Resolves the latest code version from a repository given a configurable strategy.
4. **`NotifierPlugin`** (`core/notifier.go`): Outbound alert dispatchers for deployment state changes.

---

## 🏢 Open-Core & VersionRadar Pro

VersionRadar follows an **Open-Core** model:
- **VersionRadar Community (This Repository)**: Free and open-source under Apache-2.0. Contains all 14 standard deployment providers, all 3 Git sources, all 3 version strategies, the standalone Docker container, and the complete Flutter client.
- **VersionRadar Pro**: Commercial edition for teams and enterprises, offering:
  - **Enterprise Providers**: Live Google Cloud Run revision traffic routing & percentage, Apple TestFlight internal & external beta builds.
  - **Outbound Alert Channels**: Slack attachments, Discord rich embeds, and Telegram bots.
  - **Workspaces & RBAC**: Team sharing and permission control.
  - **Dual Licensing**: Fully managed SaaS (`dash.versionradar.app`) or offline Ed25519 air-gapped self-hosted license keys.

See [docs/open-core.md](docs/open-core.md) for architectural details.

---

## 📖 Documentation

- [Provider Credentials & Setup Guide](docs/credentials.md) — Step-by-step token and permissions guide for all 14 cloud providers.
- [Open-Core Architecture](docs/open-core.md) — How Community and Pro integrate cleanly without code duplication.

---

## 📄 License

Licensed under the [Apache License, Version 2.0](LICENSE).
Copyright © 2026 VersionRadar authors.
