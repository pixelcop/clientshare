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

Development uses port 5174 and preview uses port 4174, leaving the app's port 5173 free. Scripts force the Bun runtime. The VitePress version is pinned in `package.json` and dependencies are locked in `bun.lock`.

## Publish

1. Configure your static host's root directory as `guide`.
2. Install with `bun install --frozen-lockfile`.
3. Build with `bun run --bun build`.
4. Publish `.vitepress/dist` to the documentation domain.

The proposed public address is `https://docs.clientshare.app`. DNS and hosting must be configured before that address is live. The app's `VITE_USER_GUIDE_URL` build variable can override its documentation link.

The default `.html` URLs work on ordinary static hosts without SPA fallback rules. For a subdirectory deployment, configure VitePress `base` and set the app's guide URL to match. The guide is public and requires no portal session.

## Editing

- Add task pages under `src/` and register them in `.vitepress/config.ts`.
- State the role required, use actual UI labels, and include the expected result.
- Use relative Markdown links so the build checks page destinations.
- Store screenshots in `src/public/screenshots/` with descriptive alt text.
- Use fictional demo records and public sample files. Record asset sources in `demo-assets.md`.
- Run a production build before committing. VitePress fails on broken internal links.

Upstream references: [getting started](https://vitepress.dev/guide/getting-started), [sidebar](https://vitepress.dev/reference/default-theme-sidebar), [local search](https://vitepress.dev/reference/default-theme-search), [deployment](https://vitepress.dev/guide/deploy).
