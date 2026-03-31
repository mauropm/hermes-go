BINARY    := hermes-go
MODULE    := github.com/nousresearch/hermes-go
GO        ?= go
CGO_ENABLED ?= 1

.PHONY: all build clean test vet lint run-chat run-api help

all: build

build:
	$(GO) build -o $(BINARY) .

build-static:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -ldflags="-s -w" -o $(BINARY) .

clean:
	rm -f $(BINARY)

test:
	$(GO) test -v -race -cover ./...

vet:
	$(GO) vet ./...

lint: vet

run-chat: build
	./$(BINARY) chat

run-api: build
	./$(BINARY) api

help:
	@echo "Available targets:"
	@echo "  build        Build the binary (default)"
	@echo "  build-static Build a stripped static binary"
	@echo "  clean        Remove the built binary"
	@echo "  test         Run tests with race detection and coverage"
	@echo "  vet          Run go vet"
	@echo "  lint         Alias for vet"
	@echo "  run-chat     Build and run the interactive CLI"
	@echo "  run-api      Build and run the API server"
	@echo "  help         Show this help"
