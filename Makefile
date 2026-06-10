BINARY  := shale
MODULE  := github.com/provasign/shale
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/version.Version=$(VERSION)

.PHONY: build test lint install cover clean smoke

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) ./cmd/shale

test:
	go test ./...

lint:
	golangci-lint run ./...

install:
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/shale

cover:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	go tool cover -func=coverage.out | tail -1

smoke: build
	./hack/smoke.sh

clean:
	rm -rf bin dist coverage.out coverage.html
