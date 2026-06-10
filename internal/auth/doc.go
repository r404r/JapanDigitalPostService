// Package auth 实现 Bearer token 认证中间件与 token 发行/吊销逻辑，
// token 仅以 SHA-256 hash 存储，scope 区分 read / admin。
//
// 实现见 task-0006。
package auth
