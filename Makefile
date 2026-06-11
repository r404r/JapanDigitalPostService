.PHONY: build run test lint tidy sync-soul gen web-install web-dev web-build web-test

build:
	go build -o bin/server ./cmd/server
	go build -o bin/batch  ./cmd/batch

run:
	go run ./cmd/server

test:
	go test ./...

web-install:
	npm install --prefix web

web-dev:
	npm run dev --prefix web

web-build:
	npm run build --prefix web

web-test:
	npm run test --prefix web

lint:
	gofmt -l .
	go vet ./...

tidy:
	go mod tidy

# 从 api/openapi.yaml 生成 server 代码（task-0001 接入 oapi-codegen 后启用）
gen:
	@echo "TODO(task-0001): oapi-codegen -generate types,chi-server -package api api/openapi.yaml > internal/api/api.gen.go"

# 校验两个灵魂文件一致（AGENTS.md == CLAUDE.md）
sync-soul:
	@diff -q AGENTS.md CLAUDE.md && echo "soul files in sync" || (echo "AGENTS.md and CLAUDE.md differ!" && exit 1)
