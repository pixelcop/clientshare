# Agent Development Guide

A file for [guiding coding agents](https://agents.md/).

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
