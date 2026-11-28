.PHONY: run test build vet fmt-check dist

VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)
ARCHES := amd64 arm64

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

dist:
	rm -rf dist && mkdir dist
	for arch in $(ARCHES); do \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/rwe-linux-$$arch ./cmd/rwe || exit 1; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/rwe-server-linux-$$arch ./cmd/server || exit 1; \
		CGO_ENABLED=0 GOOS=darwin GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' -o dist/rwe-darwin-$$arch ./cmd/rwe || exit 1; \
	done
	cd dist && sha256sum * > SHA256SUMS
