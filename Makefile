# Lintasan Go — build orchestration
#
# The dashboard UI (SvelteKit) is compiled to a static SPA and embedded into the
# Go binary via go:embed, so `lintasan start` serves the full app from a single
# executable with no separate Node process.
#
# ⚠️ SAFETY RULE: no build target ever writes to ./lintasan.
# ./lintasan is the path systemd executes (ExecStart=/home/ubuntu/lintasan-go/
# lintasan start). A build that writes there silently stages code into
# production — the next restart, for ANY reason (reboot, crash, another agent's
# `systemctl restart`), puts it live with no deploy step and no approval.
# It has already happened. On 9 Aug 2026 at 16:31:49 prod was serving
# v0.29.3-17-g23bbc7c; seven seconds later a stop/start issued from a different
# worktree brought up v0.29.3-16-g389ec80 — an OLDER commit — because that
# worktree's build had left its own binary at the prod path. Nobody deployed;
# prod silently went backwards for 100 seconds until the next restart.
# Builds therefore go to dist-bin/. Deploying is a separate, explicit target.
#
# Common targets:
#   make build        → frontend + embed + compile to dist-bin/lintasan  (SAFE: never touches prod)
#   make frontend     → build only the SvelteKit static output into internal/web/dist
#   make backend      → compile the Go binary into dist-bin/ (assumes dist/ already built)
#   make run          → build then run from dist-bin/ (does not touch ./lintasan)
#   make deploy       → EXPLICIT: install dist-bin/lintasan as ./lintasan + restart systemd
#   make test         → run the Go test suite (excludes the experimental provider pkg)
#   make clean        → remove build artifacts
#   make release      → cross-compile release binaries into dist-bin/

BINARY      := lintasan
PKG         := ./cmd/lintasan
DIST        := internal/web/dist
FRONTEND    := frontend
BUILD_DIR   := dist-bin
BUILD_OUT   := $(BUILD_DIR)/$(BINARY)
# The live path systemd runs. Only `make deploy` may write here.
PROD_BINARY := ./$(BINARY)
SERVICE     := lintasan
VERSION     := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X github.com/sanhaji182/lintasan-go/internal/version.Version=$(VERSION)

.PHONY: build frontend backend run test clean release deps deploy

## Full build: frontend → embed → single binary in dist-bin/ (never ./lintasan)
build: frontend backend
	@echo "✓ Built $(BUILD_OUT) ($(VERSION)) with embedded dashboard"
	@echo "  Production binary $(PROD_BINARY) was NOT modified. Run 'make deploy' to ship it."

## Compile the SvelteKit dashboard into the embedded dist directory
frontend:
	@echo "→ Building frontend (SvelteKit static SPA)…"
	cd $(FRONTEND) && npm install && npm run build
	@echo "→ Syncing build output into $(DIST)…"
	rm -rf $(DIST)
	mkdir -p $(DIST)
	cp -r $(FRONTEND)/build/* $(DIST)/
	@# keep the placeholder so the dir is never empty
	touch $(DIST)/.gitkeep

## Compile the Go binary (CGO required for go-sqlite3)
backend:
	@echo "→ Compiling Go binary → $(BUILD_OUT)…"
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BUILD_OUT) $(PKG)

## Build and start the freshly built binary (NOT the production one)
run: build
	$(BUILD_OUT) start

## Install the built binary as the production one and restart systemd.
## Deliberately separate from `build` so shipping is always a conscious act.
## Stops the service first to release the file handle ("text file busy"), and
## keeps a timestamped backup so a rollback is a copy, not a rebuild.
deploy:
	@test -x $(BUILD_OUT) || { echo "✗ $(BUILD_OUT) not found — run 'make build' first"; exit 1; }
	@echo "→ Deploying $(BUILD_OUT) ($(VERSION)) to $(PROD_BINARY)…"
	@if [ -f $(PROD_BINARY) ]; then \
		cp -p $(PROD_BINARY) $(PROD_BINARY).bak-$$(date +%Y%m%d-%H%M%S); \
		echo "  backup: $$(ls -t $(PROD_BINARY).bak-* | head -1)"; \
	fi
	sudo systemctl stop $(SERVICE)
	cp $(BUILD_OUT) $(PROD_BINARY)
	sudo systemctl start $(SERVICE)
	@sleep 1
	@sudo systemctl is-active $(SERVICE)
	@curl -s localhost:20180/health || true
	@echo ""
	@echo "✓ Deployed. Verify the version above matches $(VERSION)."

## Run tests (skip the untracked experimental provider package)
test:
	go test $$(go list ./... | grep -v '/internal/provider')

## Remove build artifacts (keeps the .gitkeep placeholder).
## Never deletes $(PROD_BINARY) — removing the file systemd executes would turn
## the next restart into an outage.
clean:
	rm -rf $(FRONTEND)/build $(FRONTEND)/.svelte-kit
	find $(DIST) -mindepth 1 ! -name '.gitkeep' -delete
	rm -rf $(BUILD_DIR)

## Cross-compile release binaries (frontend embedded). CGO needs a cross toolchain
## for non-native targets; the native target always works.
release: frontend
	@echo "→ Building release binaries into $(BUILD_DIR)/…"
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 $(PKG)
	@echo "✓ $(BUILD_DIR)/$(BINARY)-linux-amd64"

## Install frontend deps only
deps:
	cd $(FRONTEND) && npm install
