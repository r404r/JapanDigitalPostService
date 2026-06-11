.PHONY: build run test regression-report test-multidialect db-up db-down lint tidy sync-soul gen web-install web-dev web-build web-test ci openapi-lint

build:
	go build -o bin/server ./cmd/server
	go build -o bin/batch  ./cmd/batch

run:
	go run ./cmd/server

test:
	go test ./...

regression-report:
	./scripts/regression-report.sh

# 本地一键多方言测试：起 PG/MySQL，等就绪，跑 store 集成测试。
# 需要 docker。CI 用 service 容器跑同一组测试（.github/workflows/ci.yml）。
DC := docker compose -f deployments/docker-compose.yml
TEST_POSTGRES_DSN ?= postgres://postal:postal@localhost:5432/postal?sslmode=disable
TEST_MYSQL_DSN    ?= postal:postal@tcp(localhost:3306)/postal?parseTime=true&charset=utf8mb4

db-up:
	$(DC) up -d
	@echo "waiting for postgres..."; until $(DC) exec -T postgres pg_isready -U postal >/dev/null 2>&1; do sleep 1; done
	@echo "waiting for mysql...";    until $(DC) exec -T mysql mysqladmin ping -uroot -proot >/dev/null 2>&1; do sleep 1; done
	@echo "databases ready"

db-down:
	$(DC) down -v

test-multidialect: db-up
	TEST_POSTGRES_DSN="$(TEST_POSTGRES_DSN)" TEST_MYSQL_DSN="$(TEST_MYSQL_DSN)" go test -v ./internal/store/...

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
	@git diff --no-index --quiet -- AGENTS.md CLAUDE.md && echo "soul files in sync" || (echo "AGENTS.md and CLAUDE.md differ!" && git diff --no-index -- AGENTS.md CLAUDE.md; exit 1)

# OpenAPI 契约校验（需 node/npx）。
openapi-lint:
	npx --yes @redocly/cli@2.32.0 lint api/openapi.yaml --skip-rule info-license --skip-rule operation-4xx-response

# 一键本地 CI：fmt/vet/build/test + 灵魂文件 + OpenAPI + 前端（若已落地）。
ci:
	./scripts/ci.sh
