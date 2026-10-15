# Build the standalone guide at / using Bun.
FROM --platform=$BUILDPLATFORM oven/bun:1 AS docs-build
WORKDIR /app

COPY guide/package.json guide/bun.lock ./guide/
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --cwd guide --frozen-lockfile
COPY guide/src ./guide/src
COPY guide/.vitepress ./guide/.vitepress
COPY web/src/components/Logo.vue ./web/src/components/Logo.vue
RUN DOCS_LAST_UPDATED=false bun run --cwd guide --bun build

# Only Nginx and the generated static site are needed at runtime.
FROM nginx:stable-alpine AS runtime
COPY docker/docs.nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=docs-build /app/guide/.vitepress/dist/ /usr/share/nginx/html/

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1/healthz || exit 1
