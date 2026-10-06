
APP_NAME=clientshare
GO_CMD=go
GOOSE_CMD=goose
DB_PATH=./data/clientshare.db

BUILD_PATH?=
BUILD_TIME?=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT_HASH?=$(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
VITE_SENTRY_DSN?=$(shell ([ -f .build-env ] && source .build-env && echo "$$VITE_SENTRY_DSN") || echo "")
SENTRY_RELEASE?=$(COMMIT_HASH)

GO_BUILD_INFO_LDFLAGS=-X github.com/pixelcop/clientshare/internal/clientshare.BuildTime=$(BUILD_TIME) -X github.com/pixelcop/clientshare/internal/clientshare.CommitHash=$(COMMIT_HASH)
GO_BUILD_ARCH ?=
PLATFORMS ?= linux/amd64
PULL ?= false
PUSH ?= false
comma := ,

watch:
	watchexec -re go --clear --ignore-nothing -- make run

internal/static/dist:
	mkdir -p internal/static/dist
	touch internal/static/dist/.keep

internal/static/dist/index.html:
	cd web && make build-web

internal/services/email/generated/password_reset.html:
	cd web && make build-emails

run: internal/static/dist internal/services/email/generated/password_reset.html
	export DEBUG=1; \
		$(GO_CMD) run ./cmd/server/main.go

import-clients: migrate internal/services/email/generated/password_reset.html
	@test -n "$(CSV)" || (echo "CSV=/path/to/clients.csv is required" && exit 1)
	$(GO_CMD) run ./cmd/importclients -csv "$(CSV)" $(if $(INVITED_BY),-invited-by "$(INVITED_BY)",) $(if $(filter false,$(SEND_NOW)),-send-now=false,)

build:
	$(GO_BUILD_ARCH) $(GO_CMD) build -ldflags '$(GO_BUILD_INFO_LDFLAGS)' -o $(BUILD_PATH)$(APP_NAME) ./cmd/server
	$(GO_BUILD_ARCH) $(GO_CMD) build -ldflags '$(GO_BUILD_INFO_LDFLAGS)' -o $(BUILD_PATH)goose ./internal/database/migrations
	$(GO_BUILD_ARCH) $(GO_CMD) build -ldflags '$(GO_BUILD_INFO_LDFLAGS)' -o $(BUILD_PATH)importclients ./cmd/importclients

clean:
	rm -rf internal/static/dist
	rm -rf internal/services/email/generated/
	rm -f $(BUILD_PATH)$(APP_NAME)
	rm -f $(BUILD_PATH)goose
	rm -f $(BUILD_PATH)importclients

test: internal/static/dist/index.html internal/services/email/generated/password_reset.html
	$(GO_CMD) test ./...

test-web:
	cd web && make test

test-all:
	$(MAKE) test
	$(MAKE) test-web

migrate: migrate-up

migrate-up: internal/static/dist internal/services/email/generated/password_reset.html
	$(GO_CMD) run ./internal/database/migrations -dir internal/database/migrations up

migrate-down:
	$(GO_CMD) run ./internal/database/migrations -dir internal/database/migrations down

migrate-create:
	$(GOOSE_CMD) -dir internal/database/migrations create $(name) go

run-web:
	cd web && make run

generate-types:
	gorm gen -i ./internal/models/
	tygo generate

gen: generate-types

build-docker-all:
	make build-docker PLATFORMS="linux/amd64,linux/arm64"

build-docker:
		docker buildx build \
				$(if $(filter true,$(PULL)),--pull,) \
				$(if $(filter true,$(PUSH)),--push,) \
				$(if $(findstring $(comma),$(PLATFORMS)),,--load) \
				--platform $(PLATFORMS) \
				--build-arg BUILD_TIME="$(BUILD_TIME)" \
				--build-arg COMMIT_HASH="$(COMMIT_HASH)" \
				--build-arg VITE_SENTRY_DSN="$(VITE_SENTRY_DSN)" \
				--build-arg SENTRY_RELEASE="$(SENTRY_RELEASE)" \
				-f docker/Dockerfile \
				-t ghcr.io/pixelcop/clientshare:latest \
				-t ghcr.io/pixelcop/clientshare:g-$(COMMIT_HASH) \
				.

docker-test:
		docker buildx build \
				$(if $(filter true,$(PULL)),--pull,) \
				--platform $(PLATFORMS) \
				--build-arg BUILD_TIME="$(BUILD_TIME)" \
				--build-arg COMMIT_HASH="$(COMMIT_HASH)" \
				--build-arg VITE_SENTRY_DSN="$(VITE_SENTRY_DSN)" \
				--build-arg SENTRY_RELEASE="$(SENTRY_RELEASE)" \
				-f docker/Dockerfile \
				-t clientshare:test \
				--target web-test \
				.
		docker buildx build \
				$(if $(filter true,$(PULL)),--pull,) \
				--platform $(PLATFORMS) \
				--build-arg BUILD_TIME="$(BUILD_TIME)" \
				--build-arg COMMIT_HASH="$(COMMIT_HASH)" \
				--build-arg VITE_SENTRY_DSN="$(VITE_SENTRY_DSN)" \
				--build-arg SENTRY_RELEASE="$(SENTRY_RELEASE)" \
				-f docker/Dockerfile \
				-t clientshare:test \
				--target go-test \
				.

push-docker:
	docker push ghcr.io/pixelcop/clientshare:latest
	docker push ghcr.io/pixelcop/clientshare:g-$(COMMIT_HASH)

backup: ## Create a new backup of your local db
	mkdir -p data/backups/
	@set -eo pipefail; \
		read -p "name of backup: " filename; \
		d=$$(date -u +%Y%m%d%H%M%S); \
		f=$${d}; \
		if [ -n "$$filename" ]; then \
			filename=$$(echo $$filename | tr ' ' '-'); \
			f="$${f}-$${filename}"; \
		fi; \
		t="data/backups/$${f}"; \
		cp -a data/clientshare.db $${t}; \
		gzip $${t}; \
		echo "wrote db backup: $${t}.gz";

restore: ## Restore a previous backup of your local db
	@set -eo pipefail; \
		file=$$(ls -lt data/backups/*.gz | fzf --no-sort --header="select file to restore"); \
		[ -z "$$file" ] && exit 0; \
		file=$$(echo $$file | cut -d' ' -f9); \
		echo "restoring $$file"; \
		gunzip -c "$$file" > data/clientshare.db \
			&& rm -f data/clientshare.db-*

help: ## Show make target help
	@(grep -A1 '^#' $(MAKEFILE_LIST) \
		| grep -v '^--$$' \
		| while IFS= read -r line1 && IFS= read -r line2; do \
				if [[ "$$line2" =~ ^[a-zA-Z_-]+:.*$$ ]]; then \
					echo "$$line2 #$$line1"; \
				fi; \
			done; \
		grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST)) \
			| sort \
			| awk '!seen[$$1]++' \
			| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-40s\033[0m %s\n", $$1, $$2}'
