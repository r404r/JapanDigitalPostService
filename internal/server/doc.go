// Package server 装配 HTTP 路由、中间件，并把 oapi-codegen 生成的接口
// 绑定到各 service 实现（查询 / 同步状态 / token）。
//
// 骨架阶段路由内联在 cmd/server；随 task-0005 起迁入本包。
package server
