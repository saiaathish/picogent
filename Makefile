.PHONY: build test run verify-manifest

build:
	go build -o picogent ./cmd/picogent

test:
	go test ./...

run: build
	./picogent

verify-manifest:
	go run ./cmd/verify-manifest
