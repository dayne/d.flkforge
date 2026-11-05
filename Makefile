# flkforge: `make` lists what you can do.

BIN := flkforge
LINK := $(HOME)/.local/bin/flkforge
export PATH := $(HOME)/.local/go/bin:$(PATH)

.PHONY: help build install test golden check clean

help: ## what you can do (this list)
	@echo "flkforge: make TARGET"
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | awk -F'\t' '{printf "  make %-10s %s\n", $$1, $$2}'
	@echo "Installed: $$(readlink -f $(LINK) 2>/dev/null || echo 'not yet (make install)')"

build: ## build ./flkforge from this checkout
	go build -o $(BIN) ./cmd/flkforge
	@./$(BIN) version

install: build ## build, and link it as ~/.local/bin/flkforge (once; then build is enough)
	@mkdir -p $(dir $(LINK))
	ln -sfn $(CURDIR)/$(BIN) $(LINK)
	@echo "flkforge -> $(CURDIR)/$(BIN)"

test: ## run the tests, golden ones included (the flk checks skip without flk)
	go test ./...

golden: ## rewrite the golden files after an intended change (review with git diff)
	go test ./internal/cli -run Golden -update
	@git diff --stat -- testdata

check: ## what CI would run: gofmt, vet, build, race tests
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: run gofmt -w on these"; exit 1; }
	go vet ./...
	go build -o $(BIN) ./cmd/flkforge
	@if command -v gcc >/dev/null; then CGO_ENABLED=1 go test -race ./...; \
	else echo "no C compiler here: tests without -race"; go test ./...; fi

clean: ## remove the built binary
	rm -f $(BIN)
