# VersionRadar Community (Open Source)

**Every release & Git version, live on one radar.**

VersionRadar is an open-source, multi-tenant deployment and version tracker built with **Go + PocketBase** and **Flutter**. Connect your cloud providers and Git repositories with read-only credentials (AES-256-GCM encrypted) to see live build progress, deployment status, and whether what is deployed matches your latest code in Git.

## Repository Layout
- `backend/` — Go module (`github.com/danysz/versionradar/backend`) containing `core/`, `server/`, `pb_migrations/`, Community `plugins/`, Git `sources/`, and pluggable `strategies/`.
- `client/` — Flutter app (Web, macOS, iOS, Android) that dynamically adapts to all registered plugins.
- `docs/` — Provider credential guide ([docs/credentials.md](docs/credentials.md)) and Open-Core architecture ([docs/open-core.md](docs/open-core.md)).

## Quick Start (Local)

### Backend
```bash
cd backend
export VR_ENCRYPTION_KEY=$(openssl rand -base64 32)
export VR_DEV_PASSWORD_AUTH=1
go run ./cmd/server serve
```
API runs on `http://127.0.0.1:8090` (PocketBase Admin UI at `/_/`).

### Flutter Client
```bash
cd client
flutter run -d chrome --dart-define=API_URL=http://127.0.0.1:8090
```

### Self-Hosted Docker
```bash
export VR_ENCRYPTION_KEY=$(openssl rand -base64 32)
docker compose up -d
```

## Architecture & Extensibility
VersionRadar is built around four pluggable interfaces in `backend/core`:
- `ProviderPlugin` — deployment targets (Cloudflare, Heroku, GCP Cloud Build, Firebase, App Store, Google Play, plus plugin branches for HTTP Probe, Vercel, Netlify, GitHub Actions, GitLab CI, Render, Supabase, Neon).
- `SourcePlugin` — Git hosts (GitHub, GitLab, Generic HTTPS Git Smart HTTP).
- `VersionStrategy` — pluggable ways to determine the latest code version (`git_tag`, `git_branch_commit`, `git_file`).
- `NotifierPlugin` — outbound alert channels.

## License
Licensed under the [Apache License 2.0](LICENSE).
