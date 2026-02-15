VERSION    := $(shell go run ./cmd/buildver)
REMOTE_PKG := tools.zach/dev/agentcord/internal/remote
REPO_OWNER := $(shell git remote get-url origin 2>/dev/null | sed -n 's|.*github\.com[:/]\([^/]*\)/.*|\1|p')
REPO_NAME  := $(shell git remote get-url origin 2>/dev/null | sed -n 's|.*github\.com[:/][^/]*/\([^/.]*\).*|\1|p')
LDFLAGS    := -X main.version=$(VERSION) -X $(REMOTE_PKG).ldOwner=$(REPO_OWNER) -X $(REMOTE_PKG).ldRepo=$(REPO_NAME)
BIN        := agentcord$(shell go env GOEXE)

.PHONY: build test test-sh test-ps1 test-ts test-all lint generate generate-assets clean

build: generate
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BIN) ./cmd/agentcord

test:
	go test ./... -count=1

test-sh:
	bats scripts/hooks/test/unix/

test-ps1:
	pwsh -Command "Invoke-Pester scripts/hooks/test/windows/ -CI"

test-ts:
	bun test scripts/hooks/test/

test-all: test test-sh test-ts

lint:
	golangci-lint run

generate:
	go generate ./...

generate-assets:
	cd tools/generate-assets && go run .

clean:
	rm -rf dist/
