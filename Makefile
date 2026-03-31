BINARY      := hermes-go
MODULE      := github.com/nousresearch/hermes-go
GO          ?= go
CGO_ENABLED ?= 1
MIN_GO_VER  := 1.22

.PHONY: all build build-static clean test vet lint \
        run-chat run-api run-setup help \
        audit security deps go-version mod-tidy mod-verify \
        govulncheck check-vulns

all: build

## ── Build ──────────────────────────────────────────────

build: check-vulns
	$(GO) build -o $(BINARY) .

build-static: check-vulns
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -ldflags="-s -w" -o $(BINARY) .

clean:
	rm -f $(BINARY)

## ── Quality ────────────────────────────────────────────

test:
	$(GO) test -v -race -cover ./...

vet:
	$(GO) vet ./...

lint: vet

## ── Security & Vulnerability Checks ────────────────────

# Verify Go version meets minimum requirement
go-version:
	@go_ver=$$($(GO) version | grep -oE '[0-9]+\.[0-9]+' | head -1); \
	min_ver=$(MIN_GO_VER); \
	if [ "$$(printf '%s\n' "$$min_ver" "$$go_ver" | sort -V | head -1)" != "$$min_ver" ]; then \
		echo "FAIL: Go $$go_ver installed, minimum $$min_ver required"; \
		exit 1; \
	fi; \
	echo "OK: Go $$go_ver (>= $$min_ver)"

# Verify module checksums against go.sum
mod-verify:
	@echo "==> Verifying module checksums..."
	$(GO) mod verify

# Ensure go.mod and go.sum are consistent
mod-tidy:
	@echo "==> Checking module tidiness..."
	$(GO) mod tidy
	@if [ -n "$$(git diff -- go.mod go.sum 2>/dev/null)" ]; then \
		echo "FAIL: go.mod or go.sum changed after mod tidy — run 'go mod tidy' and commit"; \
		exit 1; \
	fi
	@echo "OK: go.mod and go.sum are tidy"

# Check for outdated direct and indirect dependencies
deps:
	@echo "==> Checking for outdated dependencies..."
	@echo ""
	@$(GO) list -m -u all 2>/dev/null | grep -v '$(MODULE)' \
		| awk '{ \
			if ($$2 == "" && $$3 != "") status="[UPDATE]"; \
			else if ($$3 != "") status="[UPDATE]"; \
			else status="[OK]"; \
			printf "  %-8s %-45s %s %s\n", status, $$1, $$2, $$3 \
		}'
	@echo ""
	@updates=$$($(GO) list -m -u all 2>/dev/null | grep -c '\['); \
	if [ "$$updates" -gt 0 ]; then \
		echo "WARN: $$updates dependency/dependencies have updates available"; \
		echo "      Run 'go get -u ./...' to update"; \
	else \
		echo "OK: All dependencies are up to date"; \
	fi

# Run govulncheck to find known vulnerabilities in dependencies
govulncheck:
	@echo "==> Running govulncheck..."
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Pre-build vulnerability check — warns and blocks on findings
check-vulns:
	@echo "==> Checking for known vulnerabilities..."
	@vuln_output=$$($(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1); \
	vuln_exit=$$?; \
	echo "$$vuln_output"; \
	if [ $$vuln_exit -ne 0 ]; then \
		echo ""; \
		echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
		echo " WARNING: Known vulnerabilities found in dependencies!"; \
		echo " Build blocked. Run 'make govulncheck' for details."; \
		echo " Update deps with 'go get -u ./...' and 'go mod tidy'."; \
		echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
		exit 1; \
	fi
	@echo "==> No vulnerabilities found."

# Full security audit: version + modules + vulnerabilities
audit: go-version mod-verify mod-tidy govulncheck deps
	@echo ""
	@echo "==> Security audit complete."

# Alias for audit
security: audit

## ── Run ────────────────────────────────────────────────

run-chat: build
	./$(BINARY) chat

run-api: build
	./$(BINARY) api

run-setup: build
	./$(BINARY) setup

## ── Help ───────────────────────────────────────────────

help:
	@echo "Available targets:"
	@echo ""
	@echo "  Build"
	@echo "    build          Build the binary (default)"
	@echo "    build-static   Build a stripped static binary"
	@echo "    clean          Remove the built binary"
	@echo ""
	@echo "  Quality"
	@echo "    test           Run tests with race detection and coverage"
	@echo "    vet            Run go vet"
	@echo "    lint           Alias for vet"
	@echo ""
	@echo "  Security"
	@echo "    go-version     Check Go version meets minimum ($(MIN_GO_VER))"
	@echo "    mod-verify     Verify module checksums against go.sum"
	@echo "    mod-tidy       Ensure go.mod/go.sum are consistent"
	@echo "    deps           List outdated dependencies"
	@echo "    govulncheck    Scan dependencies for known vulnerabilities"
	@echo "    audit          Run all security checks (alias: security)"
	@echo ""
	@echo "  Run"
	@echo "    run-chat       Build and run the interactive CLI"
	@echo "    run-api        Build and run the API server"
	@echo "    run-setup      Build and run the setup wizard"
	@echo ""
	@echo "  Other"
	@echo "    help           Show this help"
