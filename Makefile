BINARY      := hermes-go
MODULE      := github.com/nousresearch/hermes-go
GO          ?= go
CGO_ENABLED ?= 1
MIN_GO_VER  := 1.22

# Force Go modules for all operations
export GO111MODULE := on

# Set to 1 to skip vulnerability checks (e.g., make build FORCE=1)
FORCE ?= 0

# Set to 1 to allow build even if vulnerabilities are found (shows warning)
ALLOW_VULNS ?= 0

.PHONY: all build build-static clean test vet lint \
        run-chat run-api run-setup help \
        audit security deps go-version mod-tidy mod-verify \
        govulncheck check-vulns

all: build

## ── Build ──────────────────────────────────────────────

build:
ifeq ($(FORCE),0)
	@$(MAKE) check-vulns
endif
	$(GO) build -o $(BINARY) .

build-static:
ifeq ($(FORCE),0)
	@$(MAKE) check-vulns
endif
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
	GO111MODULE=on $(GO) mod verify

# Ensure go.mod and go.sum are consistent
mod-tidy:
	@echo "==> Checking module tidiness..."
	GO111MODULE=on $(GO) mod tidy
	@if [ -n "$$(git diff -- go.mod go.sum 2>/dev/null)" ]; then \
		echo "FAIL: go.mod or go.sum changed after mod tidy — run 'go mod tidy' and commit"; \
		exit 1; \
	fi
	@echo "OK: go.mod and go.sum are tidy"

# Check for outdated direct and indirect dependencies
deps:
	@echo "==> Checking for outdated dependencies..."
	@echo ""
	@GO111MODULE=on $(GO) list -m -u all 2>/dev/null | grep -v '$(MODULE)' \
		| awk '{ \
			if ($$2 == "" && $$3 != "") status="[UPDATE]"; \
			else if ($$3 != "") status="[UPDATE]"; \
			else status="[OK]"; \
			printf "  %-8s %-45s %s %s\n", status, $$1, $$2, $$3 \
		}'
	@echo ""
	@updates=$$(GO111MODULE=on $(GO) list -m -u all 2>/dev/null | grep -c '\['); \
	if [ "$$updates" -gt 0 ]; then \
		echo "WARN: $$updates dependency/dependencies have updates available"; \
		echo "      Run 'go get -u ./...' to update"; \
	else \
		echo "OK: All dependencies are up to date"; \
	fi

# Run govulncheck to find known vulnerabilities in dependencies
govulncheck:
	@echo "==> Running govulncheck..."
	GO111MODULE=on $(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Pre-build vulnerability check — warns and blocks on findings
check-vulns:
	@echo "==> Checking for known vulnerabilities..."
	@vuln_output=$$(GO111MODULE=on $(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1); \
	vuln_exit=$$?; \
	echo "$$vuln_output"; \
	if [ $$vuln_exit -ne 0 ]; then \
		echo ""; \
		if [ "$(ALLOW_VULNS)" = "1" ]; then \
			echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
			echo " WARNING: Known vulnerabilities found in dependencies!"; \
			echo " Continuing build (ALLOW_VULNS=1)."; \
			echo " Run 'make govulncheck' for details."; \
			echo " Update deps with 'go get -u ./...' and 'go mod tidy'."; \
			echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
		else \
			echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
			echo " WARNING: Known vulnerabilities found in dependencies!"; \
			echo " Build blocked. Run 'make govulncheck' for details."; \
			echo " Update deps with 'go get -u ./...' and 'go mod tidy'."; \
			echo " OR use 'make build FORCE=1' to skip checks."; \
			echo " OR use 'make build ALLOW_VULNS=1' to build with warnings."; \
			echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!"; \
			exit 1; \
		fi; \
	else \
		echo "==> No vulnerabilities found."; \
	fi

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
	@echo "  Build Options"
	@echo "    FORCE=1        Skip vulnerability checks (e.g., make build FORCE=1)"
	@echo "    ALLOW_VULNS=1  Build with warnings if vulns found (e.g., make build ALLOW_VULNS=1)"
	@echo ""
	@echo "  Other"
	@echo "    help           Show this help"
