# Supamanager 配置与 Supabase self-hosted/v0.8.0 对齐审计

日期：2026-09-03  
范围：Manager 的配置 DTO、服务端校验、Provisioner 渲染、配置更新流程和
`internal/templates/self-hosted-v0.8.0`。对照基准为随仓库固定的上游
`self-hosted/v0.8.0` 模板及 2026-09-03 的官方文档。

## Phase 1 status (2026-09-03)

CFG-001, CFG-002, CFG-003, CFG-004, CFG-006, CFG-007, CFG-008, CFG-009, and
CFG-010 are fixed in the Phase 1 remediation. CFG-005 remains deferred to a
future phase; the finding below is retained as historical context.

本报告只记录已确认的实现问题或产品配置缺口；没有把“未暴露每一个上游环境变量”
一概视为 bug。Manager 不提供 raw `.env` 编辑是合理的安全边界，但对外承诺的字段
必须准确地被渲染、验证并在数据变更时保持安全。

## 摘要

| 优先级 | 数量 | 结论 |
|---|---:|---|
| P1 | 6 | R2 默认不可用/不完整、Storage 切换会断开历史对象、Phone MFA 可在无短信提供商时启用、Pooler 更新不生效、Caddy 多项目端口冲突。 |
| P2 | 4 | API 密钥模式落后官方安装流程、上传限制不能配置、R2 输入校验不足、Functions 目录字段无效。 |
| 能力缺口 | 5 组 | Auth、Storage、Realtime、Functions、REST/DB/Pooler 仅覆盖受控子集；需明确产品边界或补齐。 |

## P1：应优先修复

### CFG-001：R2 没有强制 path-style

**证据**

- `StorageConfig.ForcePathStyle` 的零值为 `false`；创建项目表单也默认 `false`。
- 管理端校验只要求 R2 的 `accountId`，允许 `forcePathStyle=false`。
- 渲染器将该布尔值不加修正地写为 `GLOBAL_S3_FORCE_PATH_STYLE`。

**影响**

官方 R2/S3-compatible 示例要求 `GLOBAL_S3_FORCE_PATH_STYLE: 'true'`。用户仅填写
R2 Account ID、Bucket 与凭据并保存默认值时，可能以 virtual-hosted 风格访问 R2，
导致对象操作失败。此问题不受健康检查发现，因为 `/status` 不执行上传/下载。

**修复建议**

R2 后端不要暴露该开关：切换至 R2 时强制写入 `true`，服务端校验拒绝 `false`，
并添加 R2 上传/下载回归测试。

### CFG-002：R2 断点续传缺少必需兼容项

**证据**

渲染器没有输出 `TUS_ALLOW_S3_TAGS`；整个生产渲染路径没有针对 R2 写入该变量。

**影响**

官方文档说明 Cloudflare R2 不支持 TUS 使用的 `x-amz-tagging`；若不设
`TUS_ALLOW_S3_TAGS=false`，resumable upload 会以 HTTP 500 失败。普通小文件上传可用
会掩盖该缺陷。

**修复建议**

R2 分支固定渲染 `TUS_ALLOW_S3_TAGS=false`，并以真实 R2 或 S3 行为模拟器覆盖 TUS
创建、分片上传和下载校验。

### CFG-003：Storage 后端切换没有对象迁移或安全门禁

**证据**

配置的任何 Storage 差异都只把 `storage` 放入受影响服务并执行 Compose
`--force-recreate`；没有复制对象、验证旧/新后端、列举元数据或要求空 Storage。

**影响**

Storage 的 bucket/object 元数据保留在 Postgres，字节数据留在旧文件系统或旧 S3
bucket。切换 file→S3、S3→R2、S3 bucket→另一个 bucket 后，已有元数据仍会指向
已不存在于新后端的对象，表现为历史文件不可读。容器重建和 HTTP 健康检查成功并不
代表数据迁移成功。

**修复建议**

短期：只允许首次安装或确认 Storage 为空时切换，并显示对象数量、不可逆风险和
明确确认词。长期：单独实现可恢复的复制任务（清单、校验、切换、回滚），而不是把
迁移藏在配置保存操作内。

### CFG-004：Phone MFA 可以在没有任何短信提供商时启用

**证据**

- MFA 页面可独立把 `phoneEnrollEnabled` 与 `phoneVerifyEnabled` 设为 `true`。
- 管理端 `validateMFA` 只校验数字范围，不要求 `auth.phone.enabled`、provider 或其
  secret；渲染器也只在 `auth.phone.enabled=true` 时才注入 `GOTRUE_SMS_PROVIDER` 与
  provider 凭据。
- 官方 Phone MFA 文档明确说明它与 Phone Login 共用短信 provider 配置。

**影响**

管理员可以保存一个界面上显示为“已启用”的 Phone MFA 配置，但 Auth 容器没有发送
OTP 的 provider 或凭据；用户在 challenge 阶段无法收到验证码。这不应依赖运行时
日志才发现。

**修复建议**

当任一 Phone MFA 开关为 true 时，要求完整且已启用的 Phone provider；或者将
“短信 provider”抽成同时服务 Phone Login 与 MFA 的独立依赖，并在 MFA 页面显示其
配置状态。加入“Phone Auth 关闭 + Phone MFA 打开”被拒绝的服务端与 UI 测试。

## P2：应纳入下一轮配置改造

### CFG-005：新 API key / ES256 配置从未生成或注入

**证据**

项目密钥模型只生成旧的 HS256 `ANON_KEY` 与 `SERVICE_ROLE_KEY`。运行环境没有生成
`SUPABASE_PUBLISHABLE_KEY`、`SUPABASE_SECRET_KEY`、`JWT_KEYS`、`JWT_JWKS`；Auth、
Realtime、Storage 与 Functions 模板中启用 JWKS 的行仍保持上游默认的注释状态。

**影响**

部署会进入官方所称的 legacy-only 模式，无法使用 `sb_publishable_*` / `sb_secret_*`
和 ES256 会话 JWT，也没有对应的轮换能力。旧密钥模式可运行，因此这是兼容性和
安全演进缺口，而不是立即的启动故障。

**修复建议**

将官方 `add-new-auth-keys` 的等价逻辑纳入受控密钥生成与加密存储；在渲染阶段显式
启用 `GOTRUE_JWT_KEYS`、各服务的 `JWT_JWKS`，并提供计划性的 API key / JWK 轮换。

### CFG-006：Storage 上传上限固定为 50 MiB

**证据**

固定模板写死 `FILE_SIZE_LIMIT: 52428800`，DTO、UI、服务端校验与渲染器没有上传
限制字段。

**影响**

所有普通上传超过 50 MiB 均会失败；运营人员无法按视频、归档等业务需要调整。

**修复建议**

在 Storage 配置增加以 bytes/MB 表示的上限，设安全范围与默认值，并明确它与 bucket
级 `file_size_limit` 的关系。同步暴露必要的 TUS 限制项或在 UI 标注固定值。

### CFG-007：R2 Account ID 缺乏格式与派生 URL 校验

**证据**

R2 只校验 `accountId` 非空，然后通过字符串拼接生成 endpoint；没有把派生结果再次
作为 URL 校验，也没有限制 Cloudflare Account ID 应有的字符格式。

**影响**

输入空格、路径/查询分隔符或错误标识会产生畸形 endpoint，在运行期才失败。由于这
一后端定位为 R2 专用配置，应该比 generic S3 endpoint 有更强的输入约束。

**修复建议**

至少限制为非空的 Cloudflare account-id 字符集，并对派生 URL 调用同一 URL 校验；
错误应在保存前定位到 `storage.accountId`。

### CFG-008：Functions 的 `directory` 字段是无效配置

**证据**

- `FunctionsConfig` 默认并持久化 `directory: "./functions"`，配置变更还会触发
  Functions 重建。
- 该字段没有进入渲染器的 volume、command、environment 或 Provisioner 的函数发布
  目录；最终 Compose 始终挂载固定的 `./volumes/functions:/home/deno/functions`。
- UI 把它显示为只读，API 却仍接受并持久化任意字符串，形成“保存并重建、实际无变化”的
  死字段。

**影响**

自动化调用方或未来 UI 一旦修改该值，会得到成功的配置操作和容器重建，却不会改变
函数源代码位置；这会误导运维，也扩大了不必要的配置状态面。

**修复建议**

若目录必须由 Manager 管理，从 DTO、API 和重建 diff 中删除该字段；若要支持自定义
目录，则实现受限的项目内路径校验、Compose 挂载与发布器同一目录解析，并测试发布后
可被 Runtime 读取。

## P1：其他服务与网络配置

### CFG-009：Supavisor 的 pool size / 最大客户端连接数更新不会应用到既有租户

**证据**

- 配置变更会重建 `supavisor`，并重渲染 `POOLER_DEFAULT_POOL_SIZE` 与
  `POOLER_MAX_CLIENT_CONN`。
- 固定的上游 `volumes/pooler/pooler.exs` 只在按 `POOLER_TENANT_ID` 找不到租户时调用
  `create_tenant`；租户已存在时不执行 update。
- 两个变量恰好是该启动脚本用于创建 bootstrapped tenant 的字段，官方变量清单也说明
  其由 `pooler.exs` provisioning script 读取。

**影响**

首次安装后修改连接池大小或最大客户端数，管理器会报告 Supavisor 已重建，但持久化租户
仍保留旧值。容量调优在生产中静默失效，可能继续耗尽连接或无法放开既有连接限额。

**修复建议**

在启动/配置变更时按 tenant ID 执行幂等 update（而非仅 create-if-absent），或调用
Supavisor 管理 API 更新租户；加入“初始 20/100，更新为 40/200”后查询租户实际值的
集成测试。

### CFG-010：Caddy managed HTTPS 与多项目资源分配不兼容

**证据**

- `caddy` Compose overlay 为每个项目绑定宿主机 `80:80`、`443:443` 和 `443:443/udp`。
- Manager 允许每个项目选择 `httpsMode=caddy`，但配置资源表只预留 API、Studio、DB、
  Pooler 等端口，未预留 80/443，也未限制为单项目。
- 渲染器移除了 `container_name`，所以不会在创建阶段暴露名称冲突；第二个项目会在
  Docker 实际绑定端口时失败。

**影响**

第一个 Caddy 项目可能正常运行，第二个 Caddy 项目安装或切换 HTTPS 模式时才失败并触发
回滚。更糟的是，外部 reverse proxy 模式本来才是多项目 Manager 的默认路径，UI 目前却
把单实例上游 override 当成逐项目选项提供。

**修复建议**

短期：在 Manager 部署模型中禁用逐项目 Caddy，要求使用外部统一 reverse proxy。若产品
必须支持它，则把 Caddy 提升为宿主级单例服务，由其管理所有项目域名，或将 80/443 纳入
全局独占资源并在保存前给出清晰的单项目限制。

## 受控能力缺口（不是将所有上游变量自动判为 bug）

| 范畴 | 当前覆盖 | 尚未作为受控配置暴露的高价值能力 |
|---|---|---|
| Auth | 邮箱、SMTP、OAuth、手机、MFA、密码规则与部分 rate limit | CAPTCHA/Turnstile、SAML、Passkey/WebAuthn、session 生命周期、Auth hooks、更多速率限制与 HIBP fail-closed。 |
| Storage | file、AWS、generic S3、R2、S3 protocol、imgproxy 开关 | 上传/TUS 限制、S3 timeout/socket/multipart 调优、private asset endpoint、S3 protocol region/canonical-host 安全项、bucket 级策略。 |
| Realtime | max connections、DB pool、log level | channel/event/join 速率、并发用户/频道、复制槽和限流/heap 等保护性参数。 |
| Functions | 全局 JWT、函数变量 | per-function JWT/import map、函数资源与 outbound proxy、可观测性与运行时资源限制。 |
| REST / DB / Pooler | 少量 Postgres 与 Supavisor 调优 | PostgREST row/pool/plan 限制、Supavisor TLS、连接超时与连接保护。 |

这些条目需要产品决策：若继续以“安全的受控子集”为目标，应在界面和文档中清楚标为
未支持，而不是暗示配置页等价于完整自托管 `.env`；若目标是完整管理面，应按风险分批
加入 typed schema、服务端校验、渲染、加密存储、迁移和回归测试。

## 已确认对齐项

- S3 backend 与 Storage 的 S3-compatible API 被建模为独立开关，符合官方语义。
- `STORAGE_BACKEND`、bucket、endpoint、AWS 凭据、region、path-style 和 S3 protocol
  机密均会进入最终 Storage 容器；不是仅 UI 持久化。
- 当前使用的 `GLOBAL_S3_*` 名称是官方仍支持的 legacy aliases，短期兼容，不应单独
  作为故障处理；新字段优先使用 `STORAGE_S3_*` 是后续模板升级时的技术债。
- `General.SiteURL` 是 Manager 用来派生 `<slug>.<base-domain>` 的基域，而不是 GoTrue
  的 `SITE_URL`；渲染 `SITE_URL=https://General.Domain` 与该项目模型一致。
- Logs 使用官方 `docker-compose.logs.yml` overlay，并正确选择 Analytics 与 Vector；
  未发现 Logs 配置已保存但未渲染的字段。
- Database 的 extensions 字段在 UI 中只读且渲染前明确拒绝非空值；它是“未支持”而非
  静默忽略。

## 验证与来源

- 运行：`go test ./...`（通过）。
- 检查：`git diff --check`（通过）。
- 官方 Docker 指南：<https://supabase.com/docs/guides/self-hosting/docker>
- 官方 S3 / R2 指南：<https://supabase.com/docs/guides/self-hosting/self-hosted-s3>
- 官方 Phone Login / MFA 指南：<https://supabase.com/docs/guides/self-hosting/self-hosted-phone-mfa>
- 官方 HTTPS / Caddy 指南：<https://supabase.com/docs/guides/self-hosting/self-hosted-proxy-https>
- 官方新 API key 与 ES256 指南：<https://supabase.com/docs/guides/self-hosting/self-hosted-auth-keys>
- 固定上游版本的完整变量清单：`internal/templates/self-hosted-v0.8.0/CONFIG.md`。

## 建议修复顺序

1. CFG-001、CFG-002：修正 R2 默认渲染并补端到端 TUS 测试。
2. CFG-010：禁止逐项目 Caddy，或实现宿主级统一反向代理。
3. CFG-003、CFG-004、CFG-009：为对象切换、Phone MFA 依赖、Pooler 更新补齐安全门禁与
   真实运行态测试。
4. CFG-005：把新 API key / JWKS 设为新项目默认配置，并规划已有项目迁移。
5. CFG-006 至 CFG-008：补 Storage 的可配置限额、R2 输入防御，并移除或实现 Functions
   directory 字段。
