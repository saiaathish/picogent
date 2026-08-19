.PHONY: build test run

build:
	go build -o picogent ./cmd/picogent

test:
	go test ./...

run: build
	./picogent
