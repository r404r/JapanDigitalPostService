# API 文档

本目录存放 JapanDigitalPostService API 的人读版规格。

- OpenAPI 契约源: [`../../api/openapi.yaml`](../../api/openapi.yaml)
- V1 API 规格: [`v1.md`](v1.md)

维护规则：

1. `api/openapi.yaml` 是机器可校验的接口契约源。
2. `docs/api/v1.md` 是面向实现者、测试者与调用方的人读版 API 说明。
3. 任何 task 若改变 API 路径、认证边界、请求参数、请求体、响应体、错误码或示例，必须同步更新 `api/openapi.yaml` 与本目录文档。
4. 若 task 不涉及 API，也需要在 task 收口时明确判断“不需要更新 API 文档”。
