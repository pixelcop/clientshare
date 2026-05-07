
run:
	bun run --bun dev

dev: run

typecheck:
	bun run --bun typecheck

build-emails:
	bun run --bun build:emails

build-web:
	env VITE_SENTRY_DSN="$(VITE_SENTRY_DSN)" bun run --bun build

build: build-emails build-web

clean:
	rm -rf dist
	rm -rf ../internal/static/dist
	rm -rf ../internal/services/email/generated/

test:
	bun run --bun vitest run
