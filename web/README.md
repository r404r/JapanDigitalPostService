# Web — React sample 前端

Vite + React + TypeScript sample，用于手动验证 JapanDigitalPostService API。

## 功能

- 查询页：支持邮编、都道府県、市区町村、关键字查询，展示 `total_count` / `returned_count` / `items.length` 与地址表。
- 同步页：展示 `total_addresses`、运行状态、最近成功同步和同步历史的 `rows_total` / 错误摘要，可用 admin token 触发 full/diff。
- Token 页：发行 read/admin token、查看脱敏列表、吊销 token；明文 token 只在发行后显示一次。
- 可选载荷加密：后端返回 `X-Payload-Encryption: AES-256-GCM` 时，使用页面输入的 base64(32B) AES-GCM key 解密响应信封后再渲染。
- Bearer token 与 API 暗号化 key 只保存在当前浏览器 `sessionStorage`，不硬编码到代码中。

## 本地运行

后端默认在 `localhost:8080`，Vite dev server 会把 `/v1` 代理到后端。

```bash
make web-install
make web-dev
```

访问 `http://localhost:5173`。

若后端设置 `PAYLOAD_ENCRYPTION=aes-gcm`，请把同一个 base64(32B) `PAYLOAD_ENC_KEY`
填入页面右上角的 `API 暗号化 key`。未设置该 key 或 key 不匹配时，sample 页面会显示解密错误，
不会把加密信封当作普通 API 响应渲染。

## 验证

```bash
make web-test
make web-build
```
