module github.com/r404r/JapanDigitalPostService

go 1.22

// 骨架阶段仅依赖标准库以保证离线可编译。
// chi / oapi-codegen / gorm(+pg/mysql/sqlite) / robfig-cron 等依赖
// 在 task-0001 / task-0002 引入。
