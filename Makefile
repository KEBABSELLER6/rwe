.PHONY: run test build vet

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -race ./...

vet:
	go vet ./...
