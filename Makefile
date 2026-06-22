.PHONY: init build install test test-cover lint fmt clean changelog release-build npm-publish release test-integration test-integration-down

VERSION ?= dev
COMMIT_ID ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
ifndef BK_TE_DOMAIN
ifeq ($(wildcard bk_te_domain),)
$(warning missing required bk_te_domain file in repo root)
$(error create bk_te_domain with the TE domain value)
endif
BK_TE_DOMAIN ?= $(shell cat bk_te_domain)
endif
COMMON_LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commitID=$(COMMIT_ID) -X main.buildTime=$(BUILD_TIME)
LDFLAGS := -ldflags "$(COMMON_LDFLAGS) -X github.com/TencentBlueKing/bk-cli/internal/config.bkTEDomain=$(BK_TE_DOMAIN)"
BINARY := bk-cli
PREFIX ?= /usr/local
COVER_THRESHOLD ?= 90.0
COVER_DIR ?= .coverage
INTEGRATION_COMPOSE_FILE ?= tests/integration/compose.yaml
INTEGRATION_BK_TE_DOMAIN ?= te.example
INTEGRATION_LDFLAGS := -ldflags "$(COMMON_LDFLAGS) -X github.com/TencentBlueKing/bk-cli/internal/config.bkTEDomain=$(INTEGRATION_BK_TE_DOMAIN)"
INTEGRATION_REPORT_DIR ?= $(CURDIR)/tests/integration/artifacts/latest
INTEGRATION_PROJECT_NAME ?= bkcli-int-$(shell python3 -c 'import time, uuid; print(f"{int(time.time())}-{uuid.uuid4().hex[:6]}")')
INTEGRATION_PROJECT_NAME := $(INTEGRATION_PROJECT_NAME)
INTEGRATION_CASES_DIR ?= /workspace/tests/integration/cases
INTEGRATION_PROJECT_FILE ?= $(INTEGRATION_REPORT_DIR)/project-name.txt
INTEGRATION_BINARY_DIR ?= ./bin/integration
INTEGRATION_BKCLI_BINARY ?= $(INTEGRATION_BINARY_DIR)/bk-cli
INTEGRATION_RUNNER_BINARY ?= $(INTEGRATION_BINARY_DIR)/bk-cli-int
INTEGRATION_BKCLI_BIN ?= /workspace/bin/integration/bk-cli
INTEGRATION_RUNNER_BIN ?= /workspace/bin/integration/bk-cli-int
INTEGRATION_GOOS ?= linux
INTEGRATION_GOARCH ?= $(shell go env GOARCH)
INTEGRATION_GOARCH := $(INTEGRATION_GOARCH)

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLINES ?= $(LOCALBIN)/golines
GOFUMPT ?= $(LOCALBIN)/gofumpt
GOLINTER ?=$(LOCALBIN)/golangci-lint
GOIMPORTS ?=$(LOCALBIN)/goimports-reviser
GOIMPORTS ?=$(LOCALBIN)/ginkgo
GIT_CLIFF ?= npx --yes git-cliff

.PHONY: init
init:
	# go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(LOCALBIN) v2.11.2
	# for ginkgo
	GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@latest
	# for make mock
	GOBIN=$(LOCALBIN) go install github.com/golang/mock/mockgen@v1.6.0
	# for gofumpt
	GOBIN=$(LOCALBIN) go install mvdan.cc/gofumpt@latest
	# for golines
	GOBIN=$(LOCALBIN) go install github.com/segmentio/golines@latest
	# for goimports
	GOBIN=$(LOCALBIN) go install -v github.com/incu6us/goimports-reviser/v3@latest
	# for ginkgo
	GOBIN=$(LOCALBIN) go install -v github.com/onsi/ginkgo/v2/ginkgo
	# for release
	GOBIN=$(LOCALBIN) go install github.com/goreleaser/goreleaser/v2@latest



build:
	go build $(LDFLAGS) -o $(BINARY) .

install: build
	install -d $(PREFIX)/bin
	install -m755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "OK: $(PREFIX)/bin/$(BINARY)"

test:
	$(LOCALBIN)/ginkgo ./...

test-verbose:
	$(LOCALBIN)/ginkgo -v ./...

test-cover:
	@set -eu; \
	rm -rf "$(COVER_DIR)"; \
	mkdir -p "$(COVER_DIR)"; \
	for pkg in $$(go list ./internal/...); do \
		safe_name=$$(echo "$$pkg" | tr '/.' '__'); \
		cover_file="$(COVER_DIR)/$$safe_name.cover.out"; \
		echo "==> $$pkg"; \
		go test "$$pkg" -count=1 -coverprofile="$$cover_file"; \
		total=$$(go tool cover -func="$$cover_file" | awk '/^total:/ {sub(/%/, "", $$3); print $$3}'); \
		echo "coverage $$pkg: $$total%"; \
		awk -v total="$$total" -v threshold="$(COVER_THRESHOLD)" 'BEGIN { exit (total + 0 >= threshold + 0 ? 0 : 1) }' || { \
			echo "coverage check failed for $$pkg: $$total% < $(COVER_THRESHOLD)%"; \
			exit 1; \
		}; \
	done
	@echo "all internal packages meet the $(COVER_THRESHOLD)% coverage threshold"

lint:
	$(LOCALBIN)/golangci-lint run ./...

fmt:
	$(LOCALBIN)/golangci-lint fmt ./...


clean:
	rm -f $(BINARY)
	go clean -testcache

# Build all platform/arch combos into dist/ (requires goreleaser)
# Usage: make release-build VERSION=0.1.0
release-build:
	BK_TE_DOMAIN="$(BK_TE_DOMAIN)" GORELEASER_CURRENT_TAG=v$(VERSION) $(LOCALBIN)/goreleaser release --clean --snapshot --skip=publish
	@# Keep only archives and checksums, remove build dirs and goreleaser metadata
	rm -rf dist/bk-cli_*/
	rm -f dist/artifacts.json dist/config.yaml dist/metadata.json

# Publish npm package (run from npm/ directory)
npm-publish:
	cp README.md npm/README.md
	cd npm && npm publish
	rm -f npm/README.md

# Generate CHANGELOG.md from conventional commits
# Usage: make changelog                    (unreleased changes)
# Usage: make changelog VERSION=0.1.0      (tag a specific version)
changelog:
	@if [ "$(VERSION)" = "dev" ]; then \
		$(GIT_CLIFF) -o CHANGELOG.md; \
	else \
		$(GIT_CLIFF) --tag v$(VERSION) -o CHANGELOG.md; \
	fi

# Full release: create tag, generate changelog, set npm version, build, publish
# Usage: make release VERSION=0.1.0
release:
	@if [ "$(VERSION)" = "dev" ]; then echo "ERROR: VERSION is required, e.g. make release VERSION=0.1.0"; exit 1; fi
	git tag v$(VERSION)
	$(MAKE) changelog VERSION=$(VERSION)
	@echo '{"version":"v$(VERSION)"}' > latest.json
	cd npm && npm version $(VERSION) --no-git-tag-version --allow-same-version
	$(MAKE) release-build VERSION=$(VERSION)
	$(MAKE) npm-publish

test-integration:
	@mkdir -p "$(INTEGRATION_REPORT_DIR)" "$(INTEGRATION_BINARY_DIR)"
	GOOS="$(INTEGRATION_GOOS)" GOARCH="$(INTEGRATION_GOARCH)" go build $(INTEGRATION_LDFLAGS) -o "$(INTEGRATION_BKCLI_BINARY)" .
	GOOS="$(INTEGRATION_GOOS)" GOARCH="$(INTEGRATION_GOARCH)" go build -o "$(INTEGRATION_RUNNER_BINARY)" ./tests/integration/cmd/inttest
	@set -eu; \
	status=0; \
	printf '%s\n' "$(INTEGRATION_PROJECT_NAME)" > "$(INTEGRATION_PROJECT_FILE)"; \
	export REPO_ROOT="$(CURDIR)"; \
	export COMPOSE_PROJECT_NAME="$(INTEGRATION_PROJECT_NAME)"; \
	export BK_CLI_BIN="$(INTEGRATION_BKCLI_BIN)"; \
	export RUNNER_BIN="$(INTEGRATION_RUNNER_BIN)"; \
	export CASES_DIR="$(INTEGRATION_CASES_DIR)"; \
	export REPORT_DIR="/workspace/tests/integration/artifacts/latest"; \
	export SCENARIO="$(SCENARIO)"; \
	export CASE="$(CASE)"; \
	docker compose -f "$(INTEGRATION_COMPOSE_FILE)" up --build --abort-on-container-exit --exit-code-from test || status=$$?; \
	docker compose -f "$(INTEGRATION_COMPOSE_FILE)" logs --no-color > "$(INTEGRATION_REPORT_DIR)/compose.log" || true; \
	docker compose -f "$(INTEGRATION_COMPOSE_FILE)" down -v --remove-orphans || true; \
	exit $$status

test-integration-down:
	@set -eu; \
	project_name="$(INTEGRATION_PROJECT_NAME)"; \
	if [ -f "$(INTEGRATION_PROJECT_FILE)" ]; then \
		project_name="$$(cat "$(INTEGRATION_PROJECT_FILE)")"; \
	fi; \
	export REPO_ROOT="$(CURDIR)"; \
	export COMPOSE_PROJECT_NAME="$$project_name"; \
	docker compose -f "$(INTEGRATION_COMPOSE_FILE)" down -v --remove-orphans
