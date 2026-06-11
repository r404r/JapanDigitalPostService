.PHONY: build run test lint tidy sync-soul gen ci openapi-lint

build:
	go build -o bin/server ./cmd/server
	go build -o bin/batch  ./cmd/batch

run:
	go run ./cmd/server

test:
	go test ./...

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

# OpenAPI 契约校验（需 node/npx）。
openapi-lint:
	npx --yes @redocly/cli@latest lint api/openapi.yaml

# 一键本地 CI：fmt/vet/build/test + 灵魂文件 + OpenAPI + 前端（若已落地）。
ci:
	./scripts/ci.sh
