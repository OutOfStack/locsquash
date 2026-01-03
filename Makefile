.PHONY: build run test test-docker tag clean

VERSION ?= dev

build:
	mkdir -p bin
	go build -ldflags "-X main.version=$(VERSION)" -o bin/locsquash .

run:
	go run .

test:
	go test -v -race ./...

test-docker:
	docker build -f Dockerfile.test -t locsquash-test .
	docker run --rm locsquash-test

LINT_VERSION := v2.7
LINT_PKG := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(LINT_VERSION)
lint:
	@golangci-lint version >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install ${LINT_PKG}; }
	@echo "Found golangci-lint, running..."
	golangci-lint run
