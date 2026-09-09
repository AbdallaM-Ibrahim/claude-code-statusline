# The commands this repository is actually driven with, in one place. Each target
# mirrors a step in .github/workflows/ci.yml, release.yml or build.ps1 so that
# what runs locally is what CI runs.
#
#   make              same as `make check`
#   make check        gofmt + vet + test + build, the CI gate
#   make dist         cross-compile every release target into dist/
#   make install      build the host binary into ~/.claude
#
# Stamp a version into a build with VERSION=v1.2.3; the default answers `dev`
# from --version, like every other working-tree build.

PKG      := ./cmd/statusline
BIN      := statusline
VERSION  ?= dev
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOFLAGS  := -trimpath -ldflags "$(LDFLAGS)"

# Where the status line is installed. Claude Code reads it from settings.json,
# which the README points at ~/.claude/statusline.
INSTALL_DIR ?= $(HOME)/.claude

# The five release targets, matching release.yml and build.ps1.
TARGETS := windows/amd64 darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

.DEFAULT_GOAL := check

.PHONY: check fmt fmt-check vet test race bench vulncheck build install dist clean help

## check: gofmt, vet, test and build — what CI runs on every push
check: fmt-check vet test build

## fmt: rewrite Go files in place with gofmt
fmt:
	gofmt -l -w .

## fmt-check: fail if any Go file is not gofmt'd
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

## vet: go vet ./...
vet:
	go vet ./...

## test: the suite
test:
	go test ./...

## race: the suite under the race detector — the render fans out into goroutines
race:
	go test -race ./...

## bench: micro-benchmarks, five runs each with allocation counts
bench:
	go test ./... -run '^$$' -bench . -benchmem -count=5

## vulncheck: govulncheck over every package, as in CI and the release gate
vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## build: the host binary at ./statusline
build:
	go build $(GOFLAGS) -o $(BIN) $(PKG)

## install: build the host binary into ~/.claude (override with INSTALL_DIR=)
install:
	go build $(GOFLAGS) -o $(INSTALL_DIR)/$(BIN) $(PKG)
	@echo "installed $(INSTALL_DIR)/$(BIN)"

## dist: cross-compile every release target into dist/ with checksums
dist:
	@mkdir -p dist
	@for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; ext=""; \
		[ "$$os" = windows ] && ext=.exe; \
		out="dist/$(BIN)-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -o "$$out" $(PKG) || exit 1; \
		echo "  $$(du -h "$$out" | cut -f1)	$$out"; \
	done
	@cd dist && sha256sum $(BIN)-* > SHA256SUMS

## clean: remove the host binary and dist/
clean:
	rm -rf $(BIN) $(BIN).exe dist

## help: list the targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'
