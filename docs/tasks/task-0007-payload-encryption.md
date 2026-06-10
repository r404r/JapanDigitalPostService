# task-0007 — 可选传输载荷加密（AES-256-GCM）

- 状态: 待开始
- 依赖: task-0005, task-0006
- 阶段: 安全（高风险：加密方案，先有 ADR 见 architecture §8）

## Goal
按 architecture §8 决策实现可选的应用层响应加密；默认关闭（仅 TLS）。

## 完成条件
- [ ] `internal/crypto`：AES-256-GCM 加/解密，随机 nonce 随密文传输。
- [ ] `PAYLOAD_ENCRYPTION=none|aes-gcm` 配置开关；`none` 时行为与现状完全一致。
- [ ] 密钥从环境/KMS 注入，不入库、不硬编码；支持 key id 以便轮换。
- [ ] 文档：客户端解密约定写入 spec §6。
- [ ] 测试：往返加解密、nonce 唯一性、关闭时零开销。

## 实施边界
- 只做响应载荷封装，**不改接口语义/字段**。
- 不实现 TLS 终止（属部署层）。
- 不引入自研加密协议，仅用标准原语。

## 验证
单测往返一致；开关 off/on 回归；spec §6 与实现一致。
