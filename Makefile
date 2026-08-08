.PHONY: dev preview server web worker cli build test check check-plugin install-cli migrate-up

CONTENTCLOUD_ADDR ?= :8080
CONTENTCLOUD_WEB_DIST ?= web/dist

dev:
	./scripts/dev.sh

preview: build
	CONTENTCLOUD_ADDR=$(CONTENTCLOUD_ADDR) CONTENTCLOUD_WEB_DIST=$(CONTENTCLOUD_WEB_DIST) ./bin/contentcloud-server

server:
	go run ./cmd/contentcloud-server

web:
	pnpm --dir web dev

worker:
	go run ./cmd/contentcloud-worker

cli:
	go run ./cmd/contentcloud --help

build: build-web
	mkdir -p bin
	go build -o bin/contentcloud-server ./cmd/contentcloud-server
	go build -o bin/contentcloud-worker ./cmd/contentcloud-worker
	go build -o bin/contentcloud ./cmd/contentcloud

build-web:
	pnpm --dir web build

test:
	go test ./...
	pnpm --dir web test

check:
	go fmt ./...
	go vet ./...
	go test ./...
	pnpm architecture
	pnpm governance:v3
	pnpm governance:content
	pnpm test:plugin-signing
	pnpm check:plugin
	pnpm --dir web typecheck
	pnpm --dir web build

check-plugin:
	pnpm evaluate:plugin
	pnpm test:plugin-signing
	pnpm check:plugin

install-cli:
	go install ./cmd/contentcloud

migrate-up:
	@echo "Apply migrations/*.sql with goose against CONTENTCLOUD_DATABASE_URL"
