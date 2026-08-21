# 软件制品依赖解析与版本兼容服务 项目文档

## 一、业务目标与真实使用者
本服务面向软件发布与构建工程师，提供自研 Go 后端 API，管理软件制品、版本与依赖约束。使用者提交依赖清单后，服务完成依赖解析与版本比较，输出可安装的版本图；当存在循环依赖、版本冲突或缺失依赖时，返回结构化诊断而非静默失败。真实使用者两类：发布工程师登记制品与版本、声明依赖约束、执行发布/废弃动作并查看解析历史；构建工程师提交构建清单、获取可安装版本图、对比版本差异并排查依赖冲突。项目保持纯 API 服务形态，不引入前端页面；前端 HTML/JS/CSS 不计入规模。数据用 SQLite 持久化，制品、版本与解析请求统一留存，发布、废弃与解析等关键动作写入变更记录。

## 二、核心业务闭环
1. 发布工程师创建制品（name 全局唯一），为制品创建 semver 版本（draft）。
2. 为版本声明依赖约束（引用制品必须已存在、约束语法必须合法）。
3. 发布工程师将 draft 版本 publish；publish 时校验依赖目标完整，成功后写变更记录。
4. 构建工程师提交依赖清单 POST /resolve；服务解析每个约束、为每个制品选择最高满足版本并递归展开传递依赖。
5. 解析器检测循环依赖（输出环路径）、版本冲突（同制品互斥区间）与缺失依赖（制品不存在或无满足版本），生成可安装版本图或诊断。
6. 解析请求与结果持久化，可通过历史接口复查；关键动作全部落 change_records。
7. 版本可被 deprecate 废弃（不再参与常规解析，除非清单显式 pin）；已发布版本不可删除。

## 三、实体字段和关系
Artifact(id, name 全局唯一, description, created_at, updated_at)。
Version(id, artifact_id FK, version 语义化版本字符串, status: draft|published|deprecated, published_at, deprecated_at, created_at, updated_at)；唯一(artifact_id, version)。
Dependency(id, from_version_id FK→Version, to_artifact_id FK→Artifact, constraint 约束字符串, created_at)；唯一(from_version_id, to_artifact_id)。
ResolutionRequest(id, request_ref 业务编号唯一, manifest_json 原始清单, status: pending|running|succeeded|failed, error_code, error_message, graph_json 结果图, created_at, finished_at)。
ResolutionNode(id, request_id FK, artifact_id, version_id, depth, reason 选择原因)；反规范化便于查询结果明细。
ChangeRecord(id, entity_type, entity_id, action, before_json, after_json, created_at)；append-only。
关系：Artifact 1:N Version；Version 1:N Dependency（作为来源），Artifact 1:N Dependency（作为目标）；ResolutionRequest 1:N ResolutionNode；ChangeRecord 多态指向任意实体。

## 四、状态流转与约束
Version：draft→published（发布）→deprecated（废弃）；draft 可删除；published/deprecated 不可删除、不可回退 draft；deprecated 为终态；解析默认只选 published 且非 deprecated 版本，deprecated 仅在清单显式 pin 时可选。
ResolutionRequest：pending→running→succeeded|failed，终态不可变。
Artifact：创建后 name 不可改。
约束：制品名格式 ^[a-z0-9]([a-z0-9_-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9_-]*[a-z0-9])?)*$，长度 ≤128，全局唯一；版本必须为合法 semver；依赖约束语法合法（=、>、>=、<、<=、^、~、区间 range、|| 或、空白与），引用制品必须存在，同一版本对同一制品只能有一条声明；已发布版本不可删除。

## 五、API 输入输出和错误语义
统一错误响应 {"error":{"code":"...","message":"..."}}。状态码：200 成功；201 创建；204 删除成功；400 请求体/约束/版本/清单结构非法；404 资源不存在；409 重名、重复版本、发布态删除；422 业务校验失败；500 内部错误；503 未就绪。
GET /healthz → 200 {"status":"ok","db":"up"}，DB 不可用 503。
POST /artifacts {"name","description"} → 201 Artifact，重名 409 NAME_CONFLICT。
GET /artifacts?limit=&offset= → 分页列表。
GET /artifacts/{name} → Artifact，404。
POST /artifacts/{name}/versions {"version"} → 201 draft Version，非法 400，重复 409 VERSION_EXISTS。
GET /artifacts/{name}/versions → 列表。
POST /artifacts/{name}/versions/{version}/publish → 200 published；依赖目标缺失/约束非法 422；已发布 409。
POST /artifacts/{name}/versions/{version}/deprecate → 200 deprecated；draft 422；已废弃 409。
DELETE /artifacts/{name}/versions/{version} → 204；published/deprecated 409 CANNOT_DELETE_PUBLISHED。
PUT /artifacts/{name}/versions/{version}/dependencies {"dependencies":[{"name","constraint"}]} → 200 全量替换；引用不存在 422 DEPENDENCY_TARGET_MISSING；语法非法 400。
GET /artifacts/{name}/versions/{version}/dependencies → 列表。
POST /resolve {"manifest":[{"name","constraint"}]} → 200 {"request_id","status":"succeeded|failed","graph":[{"name","version","depth","reason"}],"diagnostics":[{"type","message","details"}]}；清单 JSON 非法或约束语法非法 400；CYCLE/CONFLICT/MISSING 为 status=failed 的诊断，不返回 5xx。
GET /resolutions?limit=&offset=&status= → 历史列表。
GET /resolutions/{id} → 详情含 graph 与 diagnostics，404。
GET /compare?left=1.2.3&right=1.2.4 → {"left","right","result":-1|0|1}，非法 400。

## 六、持久化与变更留存
SQLite（modernc.org/sqlite，纯 Go、CGO 关闭，便于交叉编译 linux/arm64 与 linux/amd64），启用 WAL 与 foreign_keys=ON，migration 维护 schema_version。表：artifacts、versions、dependencies、resolution_requests、resolution_nodes、change_records。唯一索引：artifacts.name、versions(artifact_id,version)、dependencies(from_version_id,to_artifact_id)、resolution_requests.request_ref。业务写与 change_records 在同一事务提交，变更 append-only 不可改删，供审计与问题回溯。

## 七、模块边界
cmd/server/main.go：装配 config→db→store→service→httpapi，启动 HTTP（唯一监听 :8080），优雅退出。
internal/config：环境变量（LISTEN_ADDR 默认 :8080、DB_PATH、LOG_LEVEL）。
internal/model：实体、状态枚举、制品名校验。
internal/semver：解析/校验/比较（含 prerelease、build 忽略）。
internal/constraint：约束语法 AST 解析与匹配（操作符/区间/或/与）。
internal/resolver：构建依赖图、版本选择、环检测、冲突与缺失诊断、结果排序。
internal/store：SQLite 仓储接口与实现（制品/版本/依赖/请求/结果/变更记录）。
internal/service：业务编排、状态流转校验、变更记录写入、compare 与 resolve。
internal/httpapi：路由、handler、DTO、错误映射、healthz。
internal/errcode：错误码与 HTTP 映射。
各层仅依赖下层接口，不跨层直连数据库。

## 八、关键测试
semver：合法/非法解析、prerelease 与 build 比较、稳定排序。
constraint：各操作符、区间边界、OR 组合、非法语法、匹配与淘汰。
resolver：单层最高版本选择、传递依赖展开、循环检测路径、同制品互斥区间冲突、缺失依赖、deprecated 排除与显式 pin、确定性输出。
service：制品重名、重复版本、发布态删除、依赖目标缺失、发布前校验。
store：事务内写 change_records、唯一约束、分页查询。
httpapi：各端点状态码与错误语义、healthz、错误信封。
集成：SQLite 临时库端到端（创建→发布→声明依赖→resolve→历史→compare）。

## 九、启动冒烟和验收场景
scripts/smoke_test.sh：真实启动服务（临时 DB），轮询 GET /healthz 确认 200；创建制品与版本、发布、声明依赖、POST /resolve 校验版本图与诊断；脚本用 trap 清理进程与临时数据，退出码反映结果。
Dockerfile：多阶段构建，EXPOSE 8080 暴露唯一监听端口，支持 linux/arm64 与 linux/amd64。
验收场景：1 制品重名返回 409；2 非法 semver 返回 400；3 已发布版本删除返回 409；4 依赖引用不存在制品返回 422；5 循环依赖 resolve 返回 failed+CYCLE 路径；6 同制品互斥区间返回 CONFLICT；7 缺失依赖返回 MISSING；8 compare 返回 -1/0/1；9 历史查询返回留存请求与结果；10 /healthz 200；11 Docker 双架构构建成功且容器映射任意端口可访问 API。

## 十、两阶段计划
本项目计划产出 1 条 Bug 数据；最终非测试 Go 源码严格大于 2000 且小于 5000 行、非测试 .go 文件严格大于 20 且小于 50 个；测试代码、前端 HTML/JS/CSS 等资源、vendor、空行与纯注释均不计入。
初始核心版本（目标约 1900 行非测试 Go 有效代码、约 22 个生产 .go 文件，落在 1600~2200 行、18~24 个文件区间）：覆盖 config/model/semver/constraint/resolver/store/service/httpapi/errcode/main，实现制品与版本管理、依赖约束声明、解析与诊断、版本比较、历史查询、变更留存、healthz、Dockerfile、smoke_test.sh，完整可运行、可 Docker 部署、可冒烟，不凑代码。
第一次业务扩写（目标约 3000 行、约 34 个生产 .go 文件，落在 2600~3800 行、28~40 个文件区间）：在初始版本上新增真实业务能力——锁文件生成与快照（resolve 后 pin 版本并留存可复现 lockfile 快照）、版本依赖差异对比（同一制品两个版本的依赖新增/删除/约束变更）、解析历史分页与重跑（按 request_id 复用原清单重新解析）、发布就绪检查（校验某版本全部依赖可解析并列出阻断项）、诊断明细增强（列出候选版本与逐条淘汰原因）。即使初始版本已达最终最低门槛，仍通过上述真实业务能力完成第一次扩写；不复制粘贴、不空实现、不无调用模块、不无意义包装，所有生产代码均被 API、业务服务、后台任务或启动路径真实调用。
代码质量与规模约束：不得用重复代码、死代码、空实现、无调用模块或无意义包装凑行数；每个生产文件、类型与函数都必须服务于文档中的真实业务路径并由 API、业务服务或启动流程实际使用。

本阶段最终提醒：只实现初始核心版本并按上面的初始规模分多轮、小批量写文件；不要尝试在单次响应中输出整个项目，最终交付规模留到第一次业务扩写。
