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
- Use migrations for DB schema changes; do not edit applied migrations.
- Return errors as JSON with proper HTTP status codes.
- Follow the handler -> service -> model/storage separation instead of putting business logic directly in handlers.
- Prefer existing middleware patterns for auth and role checks (`AuthRequired`, `RoleRequired`).
- Model hooks generate IDs; preserve the existing ULID/public ID patterns instead of inventing new ID formats.
- If Go models change, regenerate types with `make gen` so `web/src/types/models.ts` stays in sync.
- Keep backend changes auditable and minimal; preserve existing public routes and config shapes unless the task requires changes.
- Always create a separate Git worktree when starting a new branch, and perform that branch's work inside its worktree. Create new worktrees under `~/work/trees/`.
- Always copy `config/config.yaml` (or `.yml`) and `data/` from the main branch checkout into a new worktree before migrations or startup. Keep these local runtime files ignored. Use SQLite backup for an active database; do not overwrite an already-running worktree's database.
- After merging a branch back into its target branch, remove that branch's worktree and delete the merged local branch.

## Notes

- Run `make migrate` before starting services. The Devcraft API launcher also runs migrations before starting the backend.
- Devcraft runs `api`, `web`, and `docs` with independently allocated ports. `scripts/dev-service.sh` wires the worktree-local ports and public URLs together. Use the web public URL for the portal and docs public URL for the guide.
- Devcraft previews disable outbound email so copied pending queues cannot send real notifications.
- The API launcher binds preview domains to the tenant identified by the copied config's original `server.base_url`, and updates that tenant's public URL in the copied database only (`cmd/devpreview`).
- Required secrets such as JWT, secure-link signing, and invite token secrets must be present in config for the app to start.
- For frontend-specific rules, use the nested `web/AGENTS.md` rather than duplicating them here.

## Devcraft

- Use Devcraft to manage this project's services and worktree previews instead
  of starting separate unmanaged processes.
- Prefer the Devcraft MCP tools. Inspect projects, worktrees and status first;
  select the correct project and worktree explicitly for lifecycle and log calls.
- Automatically onboard a project into Devcraft as needed: inspect its repository
  and development commands, then register the project and its services. Reuse
  existing registrations instead of creating duplicates.
- If MCP is unavailable, use the CLI. Run `devcraft help` to discover commands
  and options rather than guessing them.
- Start all project services by default. Restart only the affected service when
  appropriate. Read service or project logs to diagnose failures.
- Update service settings directly; Devcraft restarts affected running instances
  automatically while leaving unrelated services running.
- For concurrent worktree previews, request preview hostnames and distinct ports.
  Use status to find the assigned ports and URLs; process state is not proof of
  application readiness.
- Add configurable/dynamic port support to the apps as needed (PORT environment
  variable or a command flag), and wire Devcraft's assigned port into them. Avoid
  hard-coded ports; update frontend API/HMR settings as needed for side-by-side
  worktrees. Use branch names or paths to identify the intended checkout.
- Always use public URLs for previews.
  For a public Internet/WAN-accessible HTTPS preview, set target.public=true in
  MCP for that service/worktree (and target.preview=true for a branch-specific
  hostname). Local preview hostnames alone are not publicly routable. Return
  the public_url from status to the user after checking it responds.
- Show the verified public preview URL after every turn.
- Stop only the instances you started when they are no longer needed. Do not
  stop unrelated services or the shared daemon.
