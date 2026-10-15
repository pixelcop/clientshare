# ClientShare user guide

Public documentation built with VitePress 2.0 alpha and **Bun**. Content lives in `src/`; project planning notes in `../docs/` are not published.

## Local development

From this directory:

```sh
bun install --frozen-lockfile
bun run --bun dev
bun run --bun build
bun run --bun preview
```

Development uses port 5174 and preview uses port 4174 so the guide can run alongside the app. Scripts force the Bun runtime. The VitePress version is pinned in `package.json` and dependencies are locked in `bun.lock`.

From the repository root, the equivalent commands are `make run-guide`, `make build-guide`, and `make preview-guide` (after installing dependencies).

## Documentation URL configuration

The app reads the menu URL at runtime from `server.docs_url` in `config/config.yaml`. Leave it empty to use the bundled guide in every tenancy mode:

| `tenancy.mode` | Default **User Guide** destination |
| --- | --- |
| `single` (or omitted) | `/docs/`, served by the app |
| `hosted` | `/docs/`, served by the app |

```yaml
server:
  docs_url: "" # bundled /docs/ in every tenancy mode
tenancy:
  mode: single
```

To use another site, set `server.docs_url: "https://help.example.com/"` and restart the server. HTTP(S) URLs and root-relative paths are supported. This is deployment configuration, not a per-tenant setting. No frontend rebuild is needed to change the URL; the old `VITE_USER_GUIDE_URL` build variable is no longer used.

## Bundled documentation (default)

`make run`, `make build`, and `make test` generate the embedded guide when its sources change. The Docker build also bundles it. No separate docs hosting or DNS is needed.

To regenerate explicitly from the repository root:

```sh
make build-guide-embedded
bun run --cwd guide --bun preview:embedded
```

The embedded scripts set `DOCS_EMBEDDED=true`, selecting base `/docs/` and output `internal/static/guide/dist`, which Go embeds in the binary. The embedded preview is at `http://localhost:4175/docs/`. `/docs` redirects to `/docs/`; pages, search assets, and screenshots stay under that prefix. The guide is public, and missing docs pages return a docs 404 rather than the app shell. The bundled route remains available even if the menu URL is overridden.

Docker builds disable Git-derived last-updated timestamps because the image has no repository history. Set `DOCS_LAST_UPDATED=false` when building from a source archive or another environment without Git.

In development, the backend serves the bundled guide before its frontend proxy. The frontend Vite server proxies `/docs` back to the backend, so the same menu link works from either origin. After editing guide content, rebuild the embedded guide and restart the backend, or use `make run-guide` for standalone live editing.

## Optional: publish a separate static site

1. Configure your static host's root directory as `guide`.
2. Install with `bun install --frozen-lockfile`.
3. Build with `bun run --bun build`.
4. Publish `.vitepress/dist` to the documentation domain.

Configure DNS and static hosting for your chosen domain, then explicitly set `server.docs_url` to that site's URL. The core app does not default to the public SaaS documentation site, even in hosted mode. The standalone build uses `/` as its base and stays separate from the bundled `/docs/` build.

The default `.html` URLs work on ordinary static hosts without SPA fallback rules. For a subdirectory deployment, configure VitePress `base` and set the app's guide URL to match. The guide is public and requires no portal session.

## Standalone Nginx container

Build from the repository root:

```sh
make build-docs-docker
# Or choose your own image name:
make build-docs-docker DOCS_IMAGE=registry.example.com/clientshare-docs
```

The target uses `docker/docs.Dockerfile` and tags the image with `latest` and `g-<commit>`. It supports the same `PLATFORMS`, `PULL`, and `PUSH` options as the app image build. For example, to publish both architectures:

```sh
make build-docs-docker PLATFORMS=linux/amd64,linux/arm64 PUSH=true
```

To build directly with Docker:

```sh
docker build -f docker/docs.Dockerfile -t clientshare-docs .
docker run -d --name clientshare-docs -p 8081:80 clientshare-docs
```

Open `http://localhost:8081/`. The image uses Bun only in the build stage; the runtime is Nginx serving the standalone guide at `/`. It needs no database, app configuration, volumes, or connection to the app container.

Route your documentation hostname (for example, `docs.clientshare.app`) to this container's **port 80**, and terminate HTTPS at your reverse proxy or hosting platform. Forward the path unchanged: `/` is the docs homepage. `/healthz` returns HTTP 200 for health checks. Hashed assets are cached for a year; HTML and screenshots revalidate. Missing pages return the guide's `404.html` with HTTP 404.

Configure the separate app deployment to link to it:

```yaml
server:
  docs_url: "https://docs.clientshare.app/"
```

Restart the app after changing that setting. Rebuild and redeploy the docs image when guide content changes. Omitting `server.docs_url` still defaults the app to its own bundled `/docs/` guide.

## Editing

- Add task pages under `src/` and register them in `.vitepress/config.ts`.
- State the role required, use actual UI labels, and include the expected result.
- Use relative Markdown links so the build checks page destinations.
- Store screenshots in `src/public/screenshots/` with descriptive alt text, using Markdown such as `![Describe the screen](/screenshots/example.png)`. VitePress adjusts these paths for the selected deployment base.
- Use fictional demo records and public sample files. Record asset sources in `demo-assets.md`.
- Run a production build before committing. VitePress fails on broken internal links.

## Branding and app navigation

The docs header and homepage reuse `web/src/components/Logo.vue`, the app's Sora wordmark with its original Share gradient. Client is near-black on light surfaces and near-white in dark mode. Keep the repository's `web/` directory available when building the guide; no frontend dependency installation is needed for this shared component.

The app's **User Guide** item is in the main navigation for administrators, managers, clients, and secure-link recipients, including the mobile menu. It opens the server-provided documentation URL in a new tab.

For local external-site integration testing, run the guide preview and set `server.docs_url: "http://localhost:4174"` in your development server config. To test the default in any tenancy mode, leave the override empty and use the bundled `/docs/` route.

Upstream references: [getting started](https://vitepress.dev/guide/getting-started), [sidebar](https://vitepress.dev/reference/default-theme-sidebar), [local search](https://vitepress.dev/reference/default-theme-search), [deployment](https://vitepress.dev/guide/deploy).
