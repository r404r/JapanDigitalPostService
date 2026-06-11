# task-0006 — Token 认证、发行后端与传输安全策略

- 状态: 已实现（后端能力 + 传输安全策略）
- 依赖: task-0002, task-0005（store/query 尚未落地，本任务以 domain 接口 + 进程内默认实现解耦）
- 阶段: 安全（高风险：token 安全、加密方案）

## Goal
实现 Bearer token 认证中间件、token 发行/管理后端能力，并落地传输安全策略
（默认 TLS；可选 AES-256-GCM 应用层载荷加密）。scope 区分 read/admin。

## 完成条件
- [x] 中间件校验 `Authorization: Bearer`：SHA-256 比对 `token_hash`、检查未吊销 **且未过期**、更新 `last_used_at`。
- [x] scope：read 可查询/看状态；admin 可发行/吊销/手动触发同步。失败 401，scope 不足 403。
- [x] `POST /v1/tokens`（明文仅返回一次，支持可选 `ttl_seconds` 过期）、`GET /v1/tokens`（脱敏）、`DELETE /v1/tokens/{id}`（吊销立即生效）。
- [x] 引导 admin token 来自 `ADMIN_BOOTSTRAP_TOKEN`（幂等）。
- [x] token 仅存 hash + prefix，绝不存明文。
- [x] 传输安全：基线 TLS；可选 `PAYLOAD_ENCRYPTION=none|aes-gcm`，密钥从环境注入、不入库、支持 key id 轮换；信封含 nonce/算法/kid，客户端约定写入 spec §6。
- [x] 安全错误统一为机器码 + 安全文案，绝不回显 token/hash/内部栈/配置。
- [x] 测试：缺失/无效/过期/吊销/有效 token、scope 不足、hash-only 存储、列表脱敏、加解密往返/nonce 唯一/篡改失败/关闭零开销。

## 实施边界
- 不做前端页面（task-0009）。本任务把传输加密的核心原语（`internal/crypto`，原属 task-0007）一并落地，task-0007 可在此基础上补充密钥轮换运维与 spec §6 细化。
- token 只存 hash + prefix。
- token 仓储当前为进程内默认实现（`auth.MemoryStore`，实现 `domain.TokenRepository`）；task-0002 的 GORM store 落地同一接口后无缝替换，HTTP/service 层无需改动。
- 加密只封装响应载荷，不改接口语义/字段；不实现 TLS 终止（部署层）；不自研加密协议。

## 认证边界（sample 页面 / API 管理界面）
- `GET /v1/health`：公开，无需 token。
- 数据端点（查询/同步状态）：`read` 或 `admin`。
- 管理端点（token 发行/吊销、手动触发同步）：仅 `admin`。
- React sample 的 Token 页必须持 admin token 才能发行/吊销；查询页持 read token 即可。前端不得持久化明文 token（仅会话内一次性展示新发 token）。

## 验证
`go test ./internal/auth/... ./internal/crypto/...` 覆盖鉴权矩阵与加密往返；
e2e：bootstrap → 发行 read token → 查询端点放行、管理端点被拒（403）。
