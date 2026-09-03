GO_CACHE ?= /tmp/supabase-installer-go-cache

.PHONY: test test-go test-web build lint integration verify-edge-event-worker

test: test-go test-web

test-go:
	GOCACHE=$(GO_CACHE) go test ./...

test-web:
	npm test

build:
	GOCACHE=$(GO_CACHE) go build ./...
	npm run build

lint:
	GOCACHE=$(GO_CACHE) go vet ./...
	npm run lint

integration:
	GOCACHE=$(GO_CACHE) go test -tags=integration ./tests/integration/... -v

verify-edge-event-worker:
	./scripts/verify-edge-event-worker.sh
