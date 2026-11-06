# Agent Development Guide

A file for [guiding coding agents](https://agents.md/).

## Public project documentation

- Core is a public self-hosted project. Committed documentation must serve self-hosting users, not describe our particular deployment.
- Keep deployment-specific cutover plans, host/service details, backup locations, rehearsal results and other local operational notes untracked or in the private production workspace. Do not commit them to this repository, even while there are no other users.
- General configuration, upgrade, migration, recovery and troubleshooting documentation belongs in the public repository when it applies to self-hosted installations.

## Commands

- **Run backend:** `make run`
- **Watch backend:** `make watch`
- **Build backend:** `make build`
- **Test backend:** `make test` or `go test ./...`
- **Format Go:** `gofmt -w .`
- **Lint Go:** `golangci-lint run`
- **Run migrations:** `make migrate`
- **Rollback one migration:** `make migrate-down`
- **Create migration:** `make migrate-create name=add_feature`
- **Generate DB and TS types:** `make gen`
- **Run frontend from repo root:** `make run-web`

For frontend commands and conventions under `web/`, follow `web/AGENTS.md`.

## Worktree previews

- After copying local runtime configuration into a new feature worktree, set
  `auth.hosted_passkey_origin: ""` in that worktree's ignored
  `config/config.yaml` (or `config/config.yml`) by default. Branch-specific
  preview origins do not match existing passkey relying-party/related-origin
  configuration. This disables hosted passkey login/signup for the preview and
  allows hosted email/password fallback without deleting enrolled credentials.
- When pairing a hosted control-plane preview with Core, apply this setting to
  the matching Core worktree and restart only its API through the development
  service manager. Verify hosted sign-in options report `passkey: false` while
  preserving the user's actual password availability.
- Keep the runtime override uncommitted. Never change the main checkout,
  production configuration, shared service settings or stored passkeys to make
  a feature preview work. Enable hosted passkeys only when explicitly testing
  them with a stable, correctly configured origin and relying party.

## Architecture

- `cmd/` contains entrypoints such as `server` and `importclients`.
- `internal/clientshare/` owns application startup, config loading, DB wiring, email wiring, and Fiber web server setup.
- `internal/handlers/` contains HTTP handlers and route registration.
- `internal/services/` contains business logic and storage/email abstractions.
- `internal/models/` contains GORM models and ID hooks.
- `internal/database/migrations/` contains schema migrations; treat applied migrations as immutable.
- `web/` contains the Vue app. Production frontend builds into `internal/static/dist/`, and the Go app serves that output.
- `public/` and `web/public/` hold static assets served directly.

## Conventions

- Check `config/config.yaml` before running.
- Always consult the component library's current documentation before changing UI components, props, events, focus behavior or accessibility. Use the local OpenVue MCP (`bunx @openvue/mcp`); `https://openvue.dev/mcp` is its installation guide, not a remote endpoint. Do not guess component APIs or add workarounds before checking the documented API.
- Use migrations for DB schema changes; do not edit applied migrations.
- Return errors as JSON with proper HTTP status codes.
- Follow the handler -> service -> model/storage separation instead of putting business logic directly in handlers.
- Prefer existing middleware patterns for auth and role checks (`AuthRequired`, `RoleRequired`).
- Model hooks generate IDs; preserve the existing ULID/public ID patterns instead of inventing new ID formats.
- If Go models change, regenerate types with `make gen` so `web/src/types/models.ts` stays in sync.
- Keep backend changes auditable and minimal; preserve existing public routes and config shapes unless the task requires changes.
