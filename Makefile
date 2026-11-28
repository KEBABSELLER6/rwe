.PHONY: run test build vet fmt-check vulncheck

VERSION ?= dev

run:
	go run ./cmd/server

build:
	go build -ldflags '-X main.version=$(VERSION)' -o bin/server ./cmd/server
	go build -ldflags '-X main.version=$(VERSION)' -o bin/rwe ./cmd/rwe

test:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w ."; exit 1; }

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...
