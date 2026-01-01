package main

// version is set at build time via ldflags:
//
//	go build -ldflags "-X main.version=v1.0.0"
//
// or from git tag:
//
//	go build -ldflags "-X main.version=$(git describe --tags)"
var version = "dev"
