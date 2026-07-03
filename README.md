# JapanDigitalPostService

[日本語](README.md) | [中文](README.zh-CN.md) | [English](README.en.md)

日本郵便住所検索サービス: 郵便番号データを定期同期し、token 認証付きの OpenAPI 検索 API と React sample 画面を提供します。

## ドキュメント

- アーキテクチャ設計: [`docs/architecture.md`](docs/architecture.md)
- 機能仕様: [`docs/spec.md`](docs/spec.md)
- API ドキュメント: [`docs/api/`](docs/api/)（人が読む版） / [`api/openapi.yaml`](api/openapi.yaml)（OpenAPI 契約ソース）
- UI 利用ガイド: [`docs/guide/README.ja.md`](docs/guide/README.ja.md)
- タスク分解: [`docs/tasks/`](docs/tasks/)（[索引](docs/tasks/README.md) を含む）
- Agent 作業規約: [`AGENTS.md`](AGENTS.md) / [`CLAUDE.md`](CLAUDE.md)（同一内容）

## 現状

計画済みの機能はすべて実装済みで、spec ↔ OpenAPI ↔ 実装は一致しています。

- **ストレージ層**: GORM の 3 方言（SQLite / PostgreSQL / MySQL）を実装済みです。可搬な migration SQL（`migrations/`）と複数方言の統合テストを含みます。token は永続化され、再起動しても失われません。
- **同期**: `utf_ken_all` の全量/差分エンジン（冪等、プロセス内 cron、DB ロック、保守的 fallback）、`cmd/batch` の独立入口、HTTP 手動トリガー（`POST /v1/sync/trigger`、`auto|full|diff`、非同期実行）を提供します。
- **API**: 住所検索（タイムアウト/上限/切り詰め状態）、同期状態/履歴、token 発行/管理、実行時取得設定（`GET/PUT /v1/admin/settings`、admin）を提供します。すべてのデータ端点は**実 Bearer 認証**（read/admin scope、`/v1/health` は公開）に接続済みで、任意の AES-256-GCM ペイロード暗号化も利用できます。
- **実行時設定**: 取得リトライ回数（`download_max_retry`、既定 3）、全量 URL（`scrape_full_url`）、町域名スキップ正規表現（`town_skip_regex`）をオンラインで設定でき、永続化され再起動後も保持されます（DB 上書き > env > 既定）。URL 検証は SSRF 防御を含み、正規表現は保存前に検証されます。エンジンは各同期前に設定を解決するため、再起動なしで反映されます。
- **フロントエンド**（`web/`、Vite + React + TS）: 住所検索ページ + 管理エリア（同期トリガーの自動/強制全量/強制差分、同期状態と履歴、インポートフィルター正規表現、フィルター履歴明細、token 発行/管理）を提供します。
- **品質**: 単体/統合/エンドツーエンドテスト、境界 fixture、CI（fmt/vet/build/test + 複数方言マトリクス + soul file 一致 + OpenAPI 検証）を整備済みです。

## クイックスタート

前提: Go 1.22+（OpenAPI 検証とフロントエンドには Node 20+ が必要です）。

### 1. ビルドとテスト

```bash
make build        # bin/server と bin/batch をビルド
make test         # すべての Go テストを実行（SQLite のメモリ/一時 DB）
make regression-report  # Go 回帰テストを実行し output/regression-report.txt を更新
make ci           # ローカル CI 一式: fmt + vet + build + test + soul file + OpenAPI 検証
```

`make regression-report` はプレーンテキストの回帰・カバレッジ要約を生成します。出力先は `output/regression-report.txt` で、このファイルは git 管理され、task の収束時に一緒にコミットします。一時的なカバレッジファイル `output/coverage.out` は `.gitignore` の対象です。
`make ci` は `./scripts/ci.sh` と同等で、`.github/workflows/ci.yml` の挙動と揃えています。コミット前の実行を推奨します。

### 2. 設定

すべての設定は**環境変数**です（プログラムは `.env` ファイルを自動では読み込みません。`.env.example` は変数一覧のドキュメントです。shell / systemd / コンテナ編成で注入してください）。すべてのしきい値（タイムアウト/上限/リトライ/頻度）は設定可能で、既定値は `docs/spec.md` に記載された値です。主な項目:

| 変数 | 既定 | 説明 |
|---|---|---|
| `HTTP_ADDR` | `:8080` | listen address |
| `HTTP_READ_HEADER_TIMEOUT` / `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT` | `5s` / `15s` / `30s` / `120s` | HTTP server の slow connection / read / write / keep-alive timeout |
| `DB_DRIVER` / `DB_DSN` | `sqlite` / `file:dev.db?...` | DB driver と connection string（`sqlite` \| `postgres` \| `mysql` の 3 方言を実装済み） |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | SQLite: `1` / `1` / `0s`; PG/MySQL: `25` / `10` / `1h` | GORM 書き込み経路と raw SQL 読み取り経路が共有する下位 connection pool の統一設定 |
| `SYNC_CRON` | `0 3 * * *` | プロセス内同期頻度。`SYNC_SCHEDULER_ENABLED=false` で無効化 |
| `SYNC_LOCK_RELEASE_TIMEOUT` | `5s` | 同期ロック解放 DB 操作の短い timeout |
| `QUERY_TIMEOUT` / `FUZZY_LIMIT` / `MAX_TOTAL` | `2s` / `20` / `1000` | 検索 timeout / fuzzy 上限 / too many threshold |
| `ADMIN_BOOTSTRAP_TOKEN` | — | bootstrap admin token（起動時に hash で冪等注入） |
| `PAYLOAD_ENCRYPTION` | `none` | `none`=TLS のみ（推奨）; `aes-gcm`=レスポンス body を AES-256-GCM envelope で包む |

詳細は `.env.example` と [`docs/architecture.md` §9](docs/architecture.md) を参照してください。

### 3. 郵便番号データの同期

DB が空の場合、初回同期は公式の全量 `utf_ken_all.zip`（約 12.4 万行）をダウンロードし、以後は月次差分を適用します。独立 batch 入口で手動実行できます（server と同じ engine と DB lock を共有します）。

```bash
go run ./cmd/batch --type auto    # auto = DB が空なら full、それ以外は diff
go run ./cmd/batch --type full    # 強制的に全量再構築
go run ./cmd/batch --type diff    # 強制的に差分を適用（lookback window 内の add/del）
```

server 起動後はプロセス内 cron が `SYNC_CRON` に従って自動実行します。HTTP からも手動でトリガーできます（admin token、非同期実行）。

```bash
curl -X POST localhost:8080/v1/sync/trigger \
  -H "Authorization: Bearer <admin-token>" -H "Content-Type: application/json" \
  -d '{"type":"auto"}'        # auto | full | diff。202 は解決後の running record を返す
```

各実行は `sync_runs` に記録されます（type/status/count/duration/error/skipped count）。町域名正規表現でスキップされた行は `sync_skipped_rows` に記録され、`GET /v1/sync/runs/{id}/skipped` で確認できます。

> ネットワークなしでローカル試行する場合は、`SEED_SAMPLE_DATA=true` を設定すると組み込みサンプル住所が投入され、起動直後に検索できます。

### 4. サービス起動と API 呼び出し

```bash
# bootstrap admin token を指定して server を起動
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run   # :8080 で listen

curl localhost:8080/v1/health      # 唯一の公開 endpoint。token 不要
# {"status":"ok","version":"..."}
```

#### Bearer token の設定と生成

API 呼び出しでは token を HTTP header に入れます。

```bash
Authorization: Bearer <token>
```

React 画面の `Bearer token` 入力欄には token 本体だけを入力します。例: `jdps_manual_admin_token`。`Bearer ` prefix は入力しません。

server 起動時に `ADMIN_BOOTSTRAP_TOKEN` で最初の admin token を注入できます。上のローカル起動例では次を使っています。

```bash
ADMIN_BOOTSTRAP_TOKEN=jdps_local_admin_example_token make run
```

全機能の手動テストコンテナでは、`deployments/manual-test.env` に既定値が設定されています。

```dotenv
ADMIN_BOOTSTRAP_TOKEN=jdps_manual_admin_token
PAYLOAD_ENCRYPTION=aes-gcm
PAYLOAD_ENC_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
PAYLOAD_ENC_KEY_ID=manual-test
```

そのため、`deployments/manual-test.compose.yml` で起動した後、画面の `Bearer token` 入力欄には次を直接入力できます。

```text
jdps_manual_admin_token
```

画面右上の `API 暗号化 key` 入力欄には同じ `PAYLOAD_ENC_KEY` を入力し、AES-256-GCM レスポンス復号を手動確認します。

新しい read token を生成します。

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend-read","scope":"read","ttl_seconds":86400}'
```

新しい admin token を生成します。

```bash
curl -X POST http://localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_manual_admin_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin-user","scope":"admin"}'
```

作成レスポンスの `token` field は平文 token で、一度だけ返されます。server 側には hash だけを保存するため、平文を失った場合は取得できません。既存の admin token で再発行してください。

```bash
# 検索 endpoint には read または admin scope の Bearer token が必要（token なしは 401）
T='Authorization: Bearer jdps_local_admin_example_token'

# 郵便番号で検索（1 つの郵便番号が複数町域に対応する場合があり、address_count を返す）
curl -H "$T" "localhost:8080/v1/addresses/1000001"

# fuzzy/条件検索（zipcode prefix / 都道府県 / 市区町村 / q keyword。最大 20 件 + total_count）
curl -H "$T" "localhost:8080/v1/addresses?prefecture=東京都&limit=20"

# 同期状態と履歴（read|admin）
curl -H "$T" localhost:8080/v1/sync/status
curl -H "$T" "localhost:8080/v1/sync/runs?limit=20"

# read token を発行（admin scope。平文は一度だけ返る）
curl -X POST localhost:8080/v1/tokens \
  -H "Authorization: Bearer jdps_local_admin_example_token" \
  -H "Content-Type: application/json" \
  -d '{"name":"frontend","scope":"read","ttl_seconds":86400}'

curl localhost:8080/v1/tokens -H "Authorization: Bearer jdps_local_admin_example_token"        # 一覧（mask 済み）
curl -X DELETE localhost:8080/v1/tokens/<id> -H "Authorization: Bearer jdps_local_admin_example_token"  # revoke
```

> 認証境界（spec §5.1）: `/v1/health` は公開。検索と同期状態 endpoint は `read`|`admin` が必要です。
> token 管理と `POST /v1/sync/trigger` は `admin` のみです。401 は理由を区別せず、error body に token を出しません。

### 5. フロントエンド画面（検索 + 管理）

```bash
npm install --prefix web
npm run dev --prefix web     # http://localhost:5173（dev server が backend :8080 に proxy）
```

画面には、住所検索ページ（read token）と管理エリアが含まれます。管理エリアでは、同期のトリガー（自動 / 強制全量 / 強制差分）、同期状態と履歴、`town_skip_regex` 設定、`rows_skipped` とフィルター履歴明細、token 発行/管理（admin token）を扱えます。
平文 token は作成時に一度だけ表示され、フロントエンドでは `sessionStorage` のみに保存されます。
backend を `PAYLOAD_ENCRYPTION=aes-gcm` で起動した場合、画面右上の `API 暗号化 key` 入力欄に同じ base64(32B) の `PAYLOAD_ENC_KEY` を入力してください。フロントエンドは `X-Payload-Encryption: AES-256-GCM` response header に従って JSON envelope を自動復号します。
この key も現在のブラウザの `sessionStorage` のみに保存されます。
スクリーンショット付きの操作説明は [`docs/guide/README.ja.md`](docs/guide/README.ja.md) を参照してください。

### 6. 全機能手動テスト（コンテナ）

Go backend、production build 済み React 画面、切り替え可能な PostgreSQL/MySQL service を 1 コマンドで起動します。

```bash
docker compose -f deployments/manual-test.compose.yml up -d --build
```

ブラウザで `http://localhost:8080` を開きます。既定設定は `deployments/manual-test.env` にあります。

- 既定は SQLite です。`DB_DRIVER=sqlite`、DB file は Docker volume `app-data` の `/data/manual-test.db` に永続化されます。
- 既定 bootstrap admin token は `jdps_manual_admin_token` です。ローカル手動テスト専用で、本番には使わないでください。
- 既定で AES-GCM response encryption が有効です。`PAYLOAD_ENCRYPTION=aes-gcm`、画面の `API 暗号化 key` には `manual-test.env` の `PAYLOAD_ENC_KEY` を入力してください。
- 既定で `SEED_SAMPLE_DATA=true` なので、初回起動直後に画面からサンプルデータを検索できます。実データが必要な場合は、管理ページで admin token を使い `auto`/`full` 同期をトリガーしてください。
- フロントエンドは同一 origin の `/v1` で backend を呼び出すため、追加の API base 設定は不要です。

ローカルの port が使用中の場合は、host 側 port だけを上書きします。コンテナ内設定は変えません。

```bash
APP_HOST_PORT=18080 POSTGRES_HOST_PORT=15432 MYSQL_HOST_PORT=13306 \
  docker compose -f deployments/manual-test.compose.yml up -d --build
```

DB 切り替えは `deployments/manual-test.env` の `DB_DRIVER` / `DB_DSN` だけを変更します。compose YAML や code の変更は不要です。

```dotenv
# SQLite
DB_DRIVER=sqlite
DB_DSN=file:/data/manual-test.db?cache=shared&_fk=1

# PostgreSQL（compose 内の service 名 postgres）
DB_DRIVER=postgres
DB_DSN=postgres://postal:postal@postgres:5432/postal?sslmode=disable

# MySQL（compose 内の service 名 mysql）
DB_DRIVER=mysql
DB_DSN=postal:postal@tcp(mysql:3306)/postal?parseTime=true&charset=utf8mb4
```

#### 設定変更 / コード変更時の再起動方法

`deployments/manual-test.compose.yml` は `deployments/manual-test.env` で手動テストコンテナの実行時設定を注入します。root の `.env.example` は変数一覧のドキュメントで、この compose file から自動では読み込まれません。コンテナの環境変数はコンテナ作成時に固定されるため、`deployments/manual-test.env` を変更した後は `app` コンテナを再作成する必要があります。ただし image の再 build は不要です。

```bash
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app
```

各引数の意味:

- `--no-build`: ローカルにある既存の `deployments-app` image だけを使い、Dockerfile build を起動せず、Docker Hub の base image metadata も取得しません。
- `--force-recreate`: `app` コンテナを再作成し、新しい `env_file` 設定を反映します。
- `--no-deps`: PostgreSQL / MySQL を再起動せず、無関係な service の揺れを避けます。

Go / React source を変更した場合は、`app` image の再 build が必要です（code と frontend production build が image に入ります）。通常、base image を再 pull する必要はありません。build とコンテナ再作成を分けることを推奨します。

```bash
# 1) 現在のコンテナ状態を確認
docker compose -f deployments/manual-test.compose.yml ps

# 2) app image だけを build。--pull / --no-cache を付けず、ローカル base image と cache layer をできるだけ再利用
docker compose -f deployments/manual-test.compose.yml build app

# 3) build 済みローカル image で app コンテナを再作成。PG/MySQL と app-data volume は維持
docker compose -f deployments/manual-test.compose.yml up -d --no-build --force-recreate --no-deps app

# 4) app が healthy に戻ったことを確認
docker compose -f deployments/manual-test.compose.yml ps app
curl http://localhost:8080/v1/health
```

注意:

- `deployments/manual-test.env` など実行時設定だけを変更した場合は、`--build` を使わないでください。
- `web/package*.json`、`go.mod`、`go.sum` を変更した場合、build 中に npm / Go module の network source へアクセスすることがあります。
- ローカルに base image がない場合や Docker cache を削除した場合、build は Docker Hub にアクセスする可能性があります。network が使える場合は事前に pull できます。

```bash
docker pull node:22-alpine
docker pull golang:1.22-alpine
docker pull alpine:3.20
```

次のコマンドで base image がローカルにあるか確認できます。

```bash
docker image inspect node:22-alpine golang:1.22-alpine alpine:3.20
```

オフライン build をできるだけ維持するため、cache を削除したり強制 network を使ったりする `docker system prune -a`、`docker builder prune`、`--no-cache`、`--pull` などは避けてください。

起動時に host port を上書きした場合、たとえば `APP_HOST_PORT=18080` なら health check も `curl http://localhost:18080/v1/health` に変更します。データを完全 reset する場合だけ、下の `down -v` を使います。

PostgreSQL と MySQL のデータはそれぞれ `manual-pgdata` / `manual-mysqldata` volumes に保存されます。手動テストデータを完全に削除する場合:

```bash
docker compose -f deployments/manual-test.compose.yml down -v
```

> Security note: `manual-test.env` の token、DB account、password はローカル手動テスト用の sample 値です。本番には使わないでください。実 token/password を image layer に書き込んだり、repo に commit したりしないでください。

### 7. 複数方言検証（PostgreSQL / MySQL）

```bash
docker compose -f deployments/docker-compose.yml up -d   # PG16 + MySQL8 の DB コンテナだけを起動
make test-multidialect                                    # 実 PostgreSQL/MySQL で store 統合テストを実行
```

> 注意: この compose file は**DB だけ**を提供し、アプリ本体は含みません。3 方言の一致性は CI matrix
> （`store-multidialect` job）で継続的に回帰されています。アプリを PG に接続する例:
> `DB_DRIVER=postgres DB_DSN='postgres://postal:postal@localhost:5432/postal?sslmode=disable' make run`

## テストと CI

- Go 単体/統合テストは SQLite を使い、外部依存なしで実行できます: `make test`。
- エンドツーエンド chain（同期 fixture → 検索 → token 認証）: `internal/e2e/e2e_test.go`。
- 再利用可能な境界 fixture: `internal/sync/testdata/ken_all_edgecases.csv`。
- OpenAPI 契約検証: `make openapi-lint`（`@redocly/cli`、Node が必要）。
- フロントエンドテスト: `npm run test --prefix web`（vitest）。
- CI（`.github/workflows/ci.yml`）: Go fmt/vet/build/test + PG/MySQL 複数方言 matrix + soul file 一致 + OpenAPI 検証。

## ライセンス

本リポジトリのソースコードとプロジェクトドキュメントは [Apache License 2.0](LICENSE) で提供されます。著作権者は `r404r` です。追加の attribution とデータソース境界は [`NOTICE`](NOTICE) を参照してください。

日本郵便の郵便番号データは日本郵便が提供する第三者データであり、本リポジトリはそのデータを再ライセンスしません。ダウンロードまたは利用時は、日本郵便の条件と適用法令を利用者側で確認してください。

## 技術スタック

Go + `net/http`（標準ライブラリ router）· GORM（PostgreSQL / MySQL / SQLite）· robfig/cron · 任意の AES-256-GCM ペイロード暗号化 · React (Vite + TypeScript) sample frontend（`web/`）。
