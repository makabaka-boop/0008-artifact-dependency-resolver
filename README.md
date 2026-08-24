# 软件制品依赖解析与版本兼容服务

面向软件发布与构建工程师的纯 API 后端服务：管理软件制品、版本与依赖约束，
提交依赖清单后完成依赖解析与版本比较，输出可安装的版本图，或返回循环依赖、
版本冲突、缺失依赖等结构化诊断。数据使用 SQLite 持久化，关键动作写入变更记录。

## 功能

- 制品与版本管理（semver 校验、draft→published→deprecated 状态流转）
- 依赖约束声明（操作符、区间、`||` 或、`^`/`~`）
- 依赖解析（最高版本选择、传递依赖、环检测、冲突/缺失诊断）
- 版本比较（`/compare`）
- 解析历史查询与结果留存
- 变更记录审计（append-only）
- `GET /healthz` 健康检查

## API 概览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/healthz` | 健康检查 |
| POST | `/artifacts` | 创建制品 |
| GET  | `/artifacts` | 分页列表 |
| GET  | `/artifacts/{name}` | 制品详情 |
| POST | `/artifacts/{name}/versions` | 创建 draft 版本 |
| GET  | `/artifacts/{name}/versions` | 版本列表 |
| POST | `/artifacts/{name}/versions/{version}/publish` | 发布 |
| POST | `/artifacts/{name}/versions/{version}/deprecate` | 废弃 |
| DELETE | `/artifacts/{name}/versions/{version}` | 删除 draft |
| PUT  | `/artifacts/{name}/versions/{version}/dependencies` | 全量替换依赖 |
| GET  | `/artifacts/{name}/versions/{version}/dependencies` | 依赖列表 |
| POST | `/resolve` | 提交依赖清单并解析 |
| GET  | `/resolutions` | 历史列表 |
| GET  | `/resolutions/{id}` | 解析详情 |
| GET  | `/compare?left=&right=` | 版本比较 |

统一错误信封：`{"error":{"code":"...","message":"..."}}`。

## 本地运行

```bash
go build ./...
LISTEN_ADDR=:8080 DB_PATH=./data/app.db ./server
```

## 测试与冒烟

```bash
go test -count=1 ./...
bash scripts/smoke_test.sh
```

## Docker

```bash
docker build --platform linux/arm64 -t artifact-resolver .
docker build --platform linux/amd64 -t artifact-resolver .
docker run --rm -p 8080:8080 artifact-resolver
```

`EXPOSE 8080` 为唯一监听端口，镜像同时兼容 `linux/arm64` 与 `linux/amd64`。

## 配置

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `LISTEN_ADDR` | `:8080` | 监听地址 |
| `DB_PATH` | `./data/app.db` | SQLite 数据库路径 |
| `LOG_LEVEL` | `info` | 日志级别 |
