.PHONY: build test test-web run verify-manifest

WEB_TEST_NODE ?= node

build:
	go build -o picogent ./cmd/picogent

test:
	go test ./...

test-web:
	$(WEB_TEST_NODE) --test internal/gui/web/contracts.test.js

run: build
	./picogent

verify-manifest:
	go run ./cmd/verify-manifest
