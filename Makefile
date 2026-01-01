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
