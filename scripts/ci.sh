#!/usr/bin/env bash
# 一键本地 CI：后端（fmt/vet/build/test）+ 灵魂文件一致 + OpenAPI 校验 + 前端（若已落地）。
# 与 .github/workflows/ci.yml 行为对齐，便于提交前本地复现。
#
# 用法: ./scripts/ci.sh
# 退出码非 0 即有检查未通过。
set -euo pipefail
cd "$(dirname "$0")/.."

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }

step "gofmt（无未格式化文件）"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "以下文件需要 gofmt:"; echo "$unformatted"; exit 1
fi

step "go vet"
go vet ./...

step "go build"
go build ./...

step "go test"
go test ./...

step "灵魂文件一致（AGENTS.md == CLAUDE.md）"
diff -q AGENTS.md CLAUDE.md

step "OpenAPI 校验（api/openapi.yaml）"
if command -v npx >/dev/null 2>&1; then
	npx --yes @redocly/cli@2.32.0 lint api/openapi.yaml
else
	echo "跳过：未找到 npx（CI 中由 openapi job 执行；本地装 node 后自动启用）"
fi

step "前端（web/）"
if [ -f web/package.json ]; then
	( cd web && npm ci && npm run build && npm test --if-present )
else
	echo "跳过：web/ 尚未落地（见 task-0009 React 前端）"
fi

printf '\n\033[1;32m✓ 全部检查通过\033[0m\n'
