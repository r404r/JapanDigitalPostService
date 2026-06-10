// Package version 暴露构建版本信息，可由 ldflags 注入。
package version

// Version 为服务版本，构建时可通过
// -ldflags "-X github.com/r404r/JapanDigitalPostService/internal/version.Version=x.y.z" 覆盖。
var Version = "0.1.0-dev"
