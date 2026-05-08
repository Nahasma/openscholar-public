.PHONY: build test test-verbose test-race test-cover test-cover-pkg lint

build:
	go build -o openscholar .

test:
	go test ./internal/... -timeout 120s -count=1

test-verbose:
	go test ./internal/... -timeout 120s -v -count=1

test-race:
	go test ./internal/... -timeout 180s -race -count=1

test-cover:
	go test ./internal/... -coverprofile=coverage.out -timeout 120s
	go tool cover -func=coverage.out | tail -1
	@echo "详细报告: go tool cover -html=coverage.out"

test-cover-pkg:
	@test -n "$(PKG)" || (echo "Usage: make test-cover-pkg PKG=./internal/session" && exit 1)
	go test $(PKG) -coverprofile=coverage.out -timeout 60s
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./internal/...
