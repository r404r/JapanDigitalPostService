# task-0003 — utf_ken_all 数据解析器

- 状态: 待开始
- 依赖: task-0002
- 阶段: 同步链路

## Goal
实现 Japan Post `utf_ken_all` CSV 的流式解析与规整，产出可入库的 `Address` 记录。

## 完成条件
- [ ] `internal/sync` parser：按 UTF-8 readme 字段映射（JIS 码、邮编、都道府県/市区町村/町域 汉字与カナ、标志位）。
- [ ] 流式逐行解析，不全量入内存；每行计算 `source_hash`（规整后内容 SHA-256）。
- [ ] 处理已知特例：一町域多邮编、跨多町域、丁目、"以下に掲載がない場合"等。
- [ ] 单元测试用 `testdata/` 小 fixture CSV 覆盖正常 + 特例行。

## 实施边界
- 只做解析与规整为内存记录，**不负责下载与入库**（交给 task-0004）。
- 不解析差分文件的应用逻辑，仅保证解析器能解析差分同格式行。

## 验证
`go test ./internal/sync/...`，fixture 断言字段与计数正确。
