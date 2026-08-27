# Supabase Self-hosted Web Installer & Manager PRD

**版本：** v1.0  
**产品类型：** Self-hosted Supabase 可视化安装器 / 多实例管理器  
**前端：** React / Next.js  
**底层：** 官方 Supabase Docker Compose  
**部署模型：** 一个 Project = 一套独立官方 Supabase Runtime  
**核心原则：** 不修改 Supabase 核心服务，不复制其数据库初始化逻辑，只负责配置、部署、管理和更新官方 Runtime。

---

# 1. 产品定义

本产品是一个运行在服务器上的 Web 管理器，用于：

> 通过可视化方式创建、配置、启动、修改、停止和删除多个独立的 Self-hosted Supabase Project。

它不是 Supabase 替代品，也不重新实现：

- PostgreSQL
- Auth
- PostgREST
- Realtime
- Storage
- Edge Functions
- Studio

而是把原本需要用户手工执行的：

```text
clone Supabase
→ 修改 .env
→ 修改 docker-compose
→ 配 Auth Provider
→ 配 SMTP
→ 配 Storage
→ 配 Functions
→ 配域名
→ 生成 Secrets
→ Docker Compose
→ Health Check
```

转换成：

```text
打开 Web
→ New Project
→ 可视化配置
→ Install
→ 自动完成
```

---

# 2. 为什么采用“一项目一套 Docker”

V1 明确采用：

```text
Project A
└── Official Supabase Runtime

Project B
└── Official Supabase Runtime

Project C
└── Official Supabase Runtime
```

而不是：

```text
一个 PostgreSQL
├── tenant_a
├── tenant_b
└── tenant_c
```

原因：

1. 完全沿用官方 Supabase 初始化流程；
2. Project 之间天然隔离；
3. Auth 完全隔离；
4. JWT / anon / service_role 完全隔离；
5. Storage 完全隔离；
6. 升级兼容性最高；
7. 避免自行维护 `supabase_admin`、`pgbouncer`、Realtime schema 等内部实现；
8. 后续可以逐步共享部分外围服务，但不影响 V1。

---

# 3. 官方服务范围

当前官方 Self-hosted Supabase Docker 主要包含：

- PostgreSQL
- Envoy API Gateway
- Auth / GoTrue
- PostgREST
- Realtime
- Storage
- imgproxy
- postgres-meta
- Studio
- Edge Runtime / Functions
- Supavisor
- Logflare / Analytics
- Vector

官方目前默认使用 Envoy 作为 API Gateway，并允许通过 override 使用 Kong；Logs / Analytics 已改成 opt-in，不再属于默认必须运行组件。Realtime、Storage、imgproxy、Edge Runtime 等在不需要时也可以移除以降低资源使用。

因此安装器必须覆盖上述全部组件。

---

# 4. 核心产品目标

用户能够在一个 Web UI 中：

```text
Supabase Manager

Projects
├── Bee
├── Nomo
├── Project X
└── + New Project
```

并针对每个 Project 完成：

- 创建
- 配置
- 安装
- 查看状态
- 启动
- 停止
- 重启
- 更新配置
- 更新版本
- 查看日志
- 查看 Secrets
- 配置 Auth Provider
- 配置 SMTP
- 配置 Storage
- 配置 Functions
- 配置 Realtime
- 配置连接池
- 配置 Gateway
- 配置 Studio
- 配置 HTTPS / Domain
- 删除

---

# 5. 非目标

V1 不开发：

- Supabase Cloud Billing
- 多 Region
- Kubernetes
- 多节点 HA
- Database Branching
- Supabase Managed PITR
- Supabase 官方 Platform API Clone
- Supabase Cloud Organization / Team 完整体系
- 自己实现 Auth
- 自己实现 Storage
- 自己实现 PostgreSQL 初始化
- 自己 fork Supabase Studio 做多项目

管理器只负责 Runtime orchestration。

---

# 6. 技术架构

```text
                   Browser
                      │
                 HTTPS / Web
                      │
                      ▼
              React / Next.js UI
                      │
                      ▼
               Manager Backend
                      │
         ┌────────────┼────────────┐
         │            │            │
      Config       Secrets       Jobs
         │            │            │
         └────────────┼────────────┘
                      ▼
               Provisioner
                      │
              Docker Engine API
                      │
        ┌─────────────┼──────────────┐
        │             │              │
        ▼             ▼              ▼
      Bee           Nomo          Project X
   Supabase       Supabase         Supabase
```

---

# 7. 安全架构要求

浏览器绝对不得直接：

```text
→ Docker Socket
→ Shell
→ 文件系统
```

必须：

```text
Browser
↓
Manager API
↓
Provisioner
↓
Docker
```

Docker Socket 只允许 Provisioner 访问。

---

# 8. Project 数据目录

统一：

```text
/opt/supabase-manager/
```

建议：

```text
/opt/supabase-manager/

manager/
├── config/
├── database/
├── secrets/
└── templates/

projects/
├── bee/
│   ├── project.json
│   ├── .env
│   ├── .env.functions
│   ├── docker-compose.override.yml
│   └── volumes/
│
├── nomo/
└── project-x/
```

---

# 9. Project 数据模型

```ts
Project {
  id
  name
  slug

  domain
  siteUrl

  status

  supabaseVersion

  services

  authConfig
  storageConfig
  functionsConfig
  databaseConfig
  gatewayConfig
  smtpConfig
  studioConfig

  createdAt
  updatedAt
}
```

---

# 10. 创建 Project

用户点击：

```text
+ New Project
```

进入 Wizard。

---

# 11. Step 1 — Basic

字段：

### Project Name

```text
Bee
```

### Project Slug

自动：

```text
bee
```

规则：

```text
[a-z0-9-]
```

### Domain

```text
bee.supabase.beegame.studio
```

### Site URL

业务网站：

```text
https://beegame.studio
```

### Supabase Version

默认：

```text
Recommended Stable
```

高级用户允许：

```text
Select Version
```

禁止默认使用：

```text
latest
```

必须 pin image version。

---

# 12. Step 2 — Services

所有官方服务必须出现在 UI。

分为：

## Core

### PostgreSQL

```text
Database

ON
```

默认：

**ON**

V1 不允许关闭。

---

### API Gateway

```text
API Gateway

● Envoy
○ Kong
```

默认：

```text
Envoy
```

官方当前默认 Self-hosted Gateway 为 Envoy。

V1：

Envoy 默认启用。

Kong：

Advanced。

---

### Auth

```text
Authentication

ON
```

默认：

**ON**

允许关闭，但关闭时 UI 必须提示：

```text
Supabase Auth API will not be available.
```

---

### REST API

```text
PostgREST

ON
```

默认：

**ON**

---

# 13. Management Services

## Studio

```text
Supabase Studio

ON
```

默认：

**ON**

作用：

- Table Editor
- SQL
- Authentication Users
- Storage
- Database management

---

## postgres-meta

默认：

**ON**

如果：

```text
Studio = ON
```

则强制：

```text
postgres-meta = ON
```

用户不能关闭。

---

# 14. Realtime

```text
Realtime

OFF
```

默认：

**OFF**

开启后提供：

- Postgres Changes
- Broadcast
- Presence
- WebSocket

UI：

```text
Realtime

Enable Realtime           OFF

Advanced
├── Max connections
├── DB pool
└── Log level
```

官方允许在不需要 Realtime 时从 Self-hosted stack 中移除。

---

# 15. Storage

默认：

```text
Storage = OFF
```

开启：

```text
Storage

Enable Storage           ON

Backend

● Local filesystem
○ S3 Compatible
○ AWS S3
○ Cloudflare R2
○ Custom S3
```

官方 Storage 支持：

```text
file
s3
```

作为 backend，并可以独立开启 S3-compatible API endpoint。

---

# 16. Storage — Local

配置：

```text
Backend
Local

Storage Path
/opt/supabase-manager/projects/bee/volumes/storage
```

默认自动生成。

---

# 17. Storage — S3

配置：

```text
Bucket
Region
Endpoint
Access Key
Secret Key

Force Path Style
```

映射官方：

```text
STORAGE_BACKEND=s3

GLOBAL_S3_BUCKET
GLOBAL_S3_ENDPOINT
GLOBAL_S3_FORCE_PATH_STYLE

AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
REGION
```

官方当前支持 AWS S3 和 S3-compatible provider，包括类似 Cloudflare R2 的 endpoint。

---

# 18. Storage — R2 Preset

选择：

```text
Cloudflare R2
```

UI：

```text
Account ID
Bucket
Access Key ID
Secret Access Key
Region
```

自动生成：

```text
GLOBAL_S3_ENDPOINT=
https://<account-id>.r2.cloudflarestorage.com
```

并：

```text
GLOBAL_S3_FORCE_PATH_STYLE=true
```

---

# 19. Storage S3 Protocol

独立开关：

```text
Enable S3-compatible API

OFF
```

注意：

它与：

```text
S3 Backend
```

不是一个概念。

官方明确允许：

```text
Local Backend + S3 Protocol API
```

或：

```text
S3 Backend + 无 S3 Protocol API
```

二者必须作为两个独立设置。

---

# 20. imgproxy

```text
Image Transformation

OFF
```

默认：

**OFF**

如果 Storage 未开启：

```text
imgproxy
```

自动禁用。

如果 Storage 开启：

仍保持默认 OFF。

用户需要：

```text
Image Transformation
```

时再打开。

---

# 21. Edge Functions

```text
Edge Functions

OFF
```

默认：

**OFF**

开启：

```text
Functions Runtime

ON

Default JWT verification
ON

Functions directory

/opt/.../functions
```

官方 Self-hosted Functions 使用 Edge Runtime，函数位于：

```text
volumes/functions/<function>/index.ts
```

修改代码后 restart `functions`，修改 Functions 环境变量则需要 recreate service。

---

# 22. Functions Secrets

Web Manager 提供：

```text
Functions
→ Environment Variables
```

例如：

```text
OPENAI_API_KEY
STRIPE_SECRET_KEY
RESEND_API_KEY
```

保存到：

```text
.env.functions
```

Secrets 必须：

```text
encrypted at rest
```

UI 默认：

```text
••••••••
```

---

# 23. Functions 管理

Project 页面：

```text
Functions
├── hello
├── stripe-webhook
└── ai
```

操作：

```text
Restart Runtime

Recreate Runtime

Open Functions Folder Info
```

V1 不需要实现代码编辑器。

---

# 24. Supavisor

```text
Connection Pooler

OFF
```

默认：

**OFF**

适合低流量多实例服务器。

开启：

```text
Supavisor
```

UI：

```text
Transaction Pool Port

Session Pool Port

Pool Size

Max Client Connections
```

官方 Self-hosted 默认提供 Supavisor 连接池能力。

---

# 25. Direct PostgreSQL Access

独立开关：

```text
Expose PostgreSQL Port

OFF
```

默认：

**OFF**

如果开启：

用户必须指定：

```text
Host port
```

例如：

```text
54321
```

避免多个 Project 全抢：

```text
5432
```

Manager 必须自动检测端口冲突。

---

# 26. Logs / Analytics

官方当前 Logs/Analytics 已经改成 opt-in。

因此默认：

```text
Logs & Analytics

OFF
```

开启后自动启用：

```text
Logflare
Vector
Studio Log Explorer
```

---

# 27. Logflare

```text
Analytics / Logflare

OFF
```

用户不能在：

```text
Logs = OFF
```

的情况下单独开启。

---

# 28. Vector

```text
Vector Log Collector

OFF
```

依赖：

```text
Logflare
```

自动管理。

---

# 29. Service 默认策略

最终默认：

| Service | Default |
|---|---:|
| PostgreSQL | ON |
| Envoy Gateway | ON |
| Auth | ON |
| PostgREST | ON |
| Studio | ON |
| postgres-meta | ON |
| Realtime | OFF |
| Storage | OFF |
| imgproxy | OFF |
| Edge Functions | OFF |
| Supavisor | OFF |
| Logs / Logflare | OFF |
| Vector | OFF |
| Direct DB Port | OFF |

这作为：

```text
Lightweight Preset
```

---

# 30. Presets

安装器提供三种：

## Lightweight

```text
DB
Gateway
Auth
REST
Studio
postgres-meta
```

---

## Standard

```text
Lightweight
+
Realtime
+
Storage
+
Functions
+
Supavisor
```

---

## Full

```text
全部官方服务
```

用户仍然可以手动覆盖。

---

# 31. Auth 基础设置

单独 Wizard：

```text
Authentication
```

提供：

```text
Email
Phone
Anonymous
Signup
JWT
Session
Redirect URL
OAuth
SMTP
```

---

# 32. Email Auth

默认：

```text
Email Auth ON
```

选项：

```text
Allow signup
Confirm email
Secure email change
Double confirm changes
```

---

# 33. Phone Auth

默认：

```text
OFF
```

开启后显示对应 SMS provider 配置。

V1 如果暂不支持全部 SMS 平台，可以保留：

```text
Advanced Environment Variables
```

入口。

---

# 34. Anonymous Sign-in

```text
Anonymous Sign-in

OFF
```

---

# 35. Site URL

来自 Project Basic：

```text
https://beegame.studio
```

映射：

```text
GOTRUE_SITE_URL
```

官方 Auth 依赖 Site URL 构建 email URL 和 redirect allow-list。

---

# 36. Redirect URLs

UI：

```text
Redirect URLs

https://beegame.studio/**
https://localhost:3000/**
```

支持：

```text
+
Remove
Validate
```

---

# 37. OAuth Providers

必须支持官方当前 Auth provider 列表。官方 Self-hosted OAuth 使用：

```text
GOTRUE_EXTERNAL_<PROVIDER>_*
```

配置。

V1 Provider List：

- Apple
- Azure / Microsoft
- Bitbucket
- Discord
- Facebook
- Figma
- GitHub
- GitLab
- Google
- Kakao
- Keycloak
- LinkedIn OIDC
- Notion
- Slack OIDC
- Snapchat
- Spotify
- Twitch
- Twitter/X
- WorkOS
- Zoom

---

# 38. OAuth Provider UI

例如 Google：

```text
Google

Enable Google                       ON

Client ID
[................................]

Client Secret
[••••••••••••••••••••••••••••••]

Callback URL

https://bee.supabase.beegame.studio/auth/v1/callback

[ Copy ]
```

Callback 自动生成。

不能让用户手写。

---

# 39. OAuth 配置映射

Google：

```text
GOTRUE_EXTERNAL_GOOGLE_ENABLED
GOTRUE_EXTERNAL_GOOGLE_CLIENT_ID
GOTRUE_EXTERNAL_GOOGLE_SECRET
GOTRUE_EXTERNAL_GOOGLE_REDIRECT_URI
```

官方目前 Self-hosted Provider 就通过这些环境变量配置，并在修改后 recreate Auth 服务。

---

# 40. Provider 特殊字段

安装器必须支持 Provider-specific configuration。

例如：

### Azure

```text
Tenant URL
```

### GitHub Enterprise

```text
URL
```

### GitLab Self-hosted

```text
URL
```

### Keycloak

```text
Realm URL
```

官方 Provider 列表存在这些额外 URL 参数。

---

# 41. Provider 配置验证

保存后：

```text
Update project config
↓
Recreate Auth
↓
Wait healthy
↓
GET /auth/v1/settings
↓
Verify provider
```

官方建议通过：

```text
/auth/v1/settings
```

确认 provider 是否开启。

---

# 42. Provider Rollback

如果：

```text
Auth recreate
↓
unhealthy
```

Manager 必须：

```text
restore previous config
↓
recreate auth
↓
health check
```

用户看到：

```text
Configuration failed.

Previous working configuration has been restored.
```

---

# 43. SMTP

默认：

```text
Custom SMTP

OFF
```

配置：

```text
Host
Port
Username
Password

Sender email
Sender name
```

Password 加密保存。

---

# 44. Database Configuration

页面：

```text
Database
```

配置：

```text
Postgres Version

Database Password

Expose Direct Port

Max Connections

Shared Buffers

Extensions
```

V1 高级 Postgres 参数可以统一放：

```text
Advanced
```

---

# 45. Database Password

默认：

```text
Auto Generate
```

提供：

```text
Reveal
Copy
Rotate
```

Rotate Password 属于敏感 operation。

必须明确 warning。

---

# 46. API Keys

安装器自动生成：

```text
JWT_SECRET

ANON_KEY

SERVICE_ROLE_KEY
```

页面：

```text
Settings
→ API
```

显示：

```text
Project URL

Anon Key

Service Role Key
```

Service Role 默认：

```text
Hidden
```

---

# 47. Secrets

以下必须加密：

```text
Database password
JWT secret
Service Role key
OAuth secrets
SMTP password
S3 secret
Functions secrets
```

Manager Database 不保存明文。

---

# 48. Gateway

页面：

```text
API Gateway
```

配置：

```text
Gateway Type

● Envoy
○ Kong
```

默认：

Envoy。

Gateway 负责统一暴露：

```text
/rest/v1
/auth/v1
/storage/v1
/realtime/v1
/functions/v1
```

官方架构即由 API Gateway 对这些内部服务进行路由。

---

# 49. Domain

Project：

```text
bee.supabase.beegame.studio
```

Manager：

```text
supabase-manager.beegame.studio
```

建议使用：

```text
*.supabase.beegame.studio
```

Wildcard DNS。

---

# 50. HTTPS

Manager 支持三种模式：

```text
External Reverse Proxy

Caddy Managed

Manual Certificate
```

默认推荐：

```text
External Reverse Proxy
```

如果服务器已有：

```text
Nginx
Cloudflare
```

则 Manager 只生成 upstream 配置。

---

# 51. Reverse Proxy

每 Project 使用独立内部端口。

Manager 自动维护：

```text
bee.supabase.beegame.studio
→ 127.0.0.1:18001

nomo.supabase.beegame.studio
→ 127.0.0.1:18002
```

不能让每个 Supabase Stack 都争：

```text
8000
```

---

# 52. Port Allocator

Manager 必须拥有端口分配器。

例如：

```text
Project API:

18001
18002
18003
...
```

Studio / DB direct port 同样自动分配。

端口不能手工冲突。

---

# 53. Install Review

安装前显示：

```text
Bee

Domain
bee.supabase.beegame.studio

Preset
Lightweight

Enabled Services
Database
Gateway
Auth
REST
Studio
postgres-meta

Disabled
Realtime
Storage
imgproxy
Functions
Supavisor
Logs

Authentication
Email
Google

Storage
Disabled

Estimated Containers
6
```

---

# 54. Install 流程

点击：

```text
Install
```

创建 Operation。

流程：

```text
1 Validate host
2 Validate Docker
3 Validate disk
4 Validate ports
5 Validate domain
6 Generate secrets
7 Create project directory
8 Copy official Supabase template
9 Apply pinned image versions
10 Generate .env
11 Generate service configuration
12 Generate compose override
13 Configure Auth
14 Configure Storage
15 Configure Functions
16 Allocate ports
17 Create Docker project
18 Start PostgreSQL
19 Wait DB healthy
20 Start dependent services
21 Wait services healthy
22 Validate Gateway
23 Validate Auth
24 Validate REST
25 Validate Studio
26 Validate optional services
27 Register reverse proxy
28 Final health check
29 Mark running
```

---

# 55. 安装进度 UI

```text
Installing Bee

✓ Validate server
✓ Generate secrets
✓ Prepare Supabase
✓ PostgreSQL started
✓ Auth started
✓ REST started
● Starting Studio
○ Verify
○ Complete
```

支持：

```text
Show details
```

---

# 56. Operation Model

```ts
Operation {
  id
  projectId

  type

  status

  currentStep
  progress

  startedAt
  finishedAt

  logs
  error
}
```

Operation 类型：

```text
CREATE
START
STOP
RESTART
UPDATE_CONFIG
UPDATE_VERSION
DELETE
BACKUP
RESTORE
```

---

# 57. 安装失败

不能留下未知半成品。

失败后：

```text
Installation failed.

Step:
Start Auth

Error:
...

[Retry]

[Rollback]

[View Logs]
```

---

# 58. Rollback

创建失败：

优先：

```text
stop containers
remove temporary containers
remove network
```

数据库 data 默认不自动删除。

用户选择：

```text
Delete failed project data
```

才删除。

---

# 59. Project Dashboard

首页：

```text
Bee

Healthy

API
https://bee.supabase.beegame.studio

Services

Database       Running
Gateway        Running
Auth           Running
REST           Running
Studio         Running
Realtime       Disabled
Storage        Disabled
Functions      Disabled
```

---

# 60. Service Management

每项服务：

```text
Auth

Running

CPU
RAM

Restart
Recreate
Logs
Configure
```

---

# 61. 修改配置

例如：

```text
Realtime OFF → ON
```

Manager：

```text
update project.json
↓
update compose configuration
↓
start realtime
↓
health check
```

无需重装 Project。

---

# 62. Auth Provider 修改

```text
Google OFF → ON
```

只：

```text
recreate auth
```

不重启：

```text
Postgres
REST
Storage
Realtime
```

---

# 63. Storage 修改

Storage：

```text
OFF → ON
```

启动：

```text
storage
```

如果：

```text
imgproxy ON
```

同时启动 imgproxy。

---

# 64. Functions 修改

Functions：

```text
OFF → ON
```

启动：

```text
functions
```

修改 Functions env：

```text
recreate functions
```

修改 code：

```text
restart functions
```

符合官方当前 Self-hosted Functions 生命周期。

---

# 65. Logs 修改

```text
Logs OFF → ON
```

自动应用官方：

```text
logs override
```

启动：

```text
Logflare
Vector
```

并启用 Studio Log Explorer。

官方现在使用 optional logs compose 配置。

---

# 66. Project Start

```text
Start
```

按照依赖顺序：

```text
Database
↓
Gateway dependencies
↓
Auth / REST
↓
Optional services
↓
Studio
```

---

# 67. Project Stop

```text
Stop
```

保留：

```text
Volumes
Config
Secrets
Metadata
```

释放：

```text
CPU
RAM
```

这是多项目低频使用场景的重要能力。

---

# 68. Project Restart

支持：

```text
Restart All
```

以及：

```text
Restart Service
```

---

# 69. Project Delete

必须输入：

```text
PROJECT NAME
```

二次确认。

提供：

```text
Delete runtime only

Delete runtime + data
```

默认：

```text
保留数据
```

更安全。

---

# 70. Health

状态：

```text
Healthy

Degraded

Starting

Stopped

Unhealthy

Unknown
```

---

# 71. Health 判断

Project Healthy：

```text
DB healthy
Gateway healthy
所有 Enabled Service healthy
```

如果：

```text
Realtime unhealthy
```

但核心服务正常：

```text
Project = Degraded
```

---

# 72. Logs

UI：

```text
Logs

Service
[ Auth ▼ ]

Search
[................]

Live Tail
```

V1 可以直接读取 Docker logs。

---

# 73. Sensitive Log Redaction

必须自动过滤：

```text
Authorization
apikey
JWT_SECRET
SERVICE_ROLE
POSTGRES_PASSWORD
OAuth Secret
SMTP Password
AWS Secret
Functions secrets
```

---

# 74. Config 页面

显示最终：

```text
Services

Auth

Storage

Functions

Network

Secrets

Versions
```

不要直接让普通用户编辑 Compose。

---

# 75. Advanced Environment Variables

高级用户可使用：

```text
Advanced
→ Environment Variables
```

追加：

```text
KEY
VALUE
SERVICE
```

用于覆盖 Manager 尚未支持的新 Supabase 配置。

这是保证未来兼容官方新功能的重要机制。

---

# 76. 配置优先级

```text
Official defaults
↓
Manager generated values
↓
User UI config
↓
Advanced overrides
```

---

# 77. Version Management

页面：

```text
Supabase Runtime

Current
2026.xx

Available
2026.yy
```

Manager 不能：

```text
docker pull latest
```

直接升级生产环境。

---

# 78. Upgrade 流程

```text
Backup config
↓
Pull images
↓
Check migration notes
↓
Upgrade
↓
Health check
↓
Success
```

失败：

```text
Rollback image version
```

数据库 breaking migration 不能保证自动回滚，必须提前警告。

---

# 79. Template Version

Manager 自己维护：

```text
Supported Supabase Template Version
```

例如：

```text
manager template v1
→ compatible with Supabase X
```

避免 Compose 新版本突然 breaking。

---

# 80. Import Existing Project

V1.1 建议支持：

```text
Import Existing Supabase
```

选择：

```text
Project Directory
```

Manager 读取：

```text
.env
compose
containers
```

并生成 project metadata。

---

# 81. Backup

虽然 Supabase Self-hosted 不提供 Managed Backup/PITR，Manager 可以提供基础备份能力。官方 Self-hosted 与 Platform 的 managed backup 能力存在差异。

V1：

```text
Database pg_dump
Project Config
Secrets backup
Functions
```

---

# 82. Backup Destination

支持：

```text
Local

S3 Compatible
```

后续：

```text
R2
```

---

# 83. Backup Encryption

Backup 中包含敏感数据。

必须支持：

```text
encrypted backup
```

---

# 84. Manager Database

Manager 自己使用：

```text
SQLite
```

或：

```text
PostgreSQL
```

V1 建议：

```text
SQLite
```

原因：

- 简单；
- 单节点；
- 不增加额外服务；
- 容易备份。

后续可迁 PostgreSQL。

---

# 85. Manager 数据模型

至少：

```text
projects

project_services

project_config

project_secrets

operations

ports

versions

backups
```

---

# 86. Secret Encryption

Manager 启动时必须有：

```text
MASTER_ENCRYPTION_KEY
```

所有秘密：

```text
AES-256-GCM
```

或可靠 secret-store 加密。

禁止：

```text
明文保存数据库
```

---

# 87. Manager 登录

V1：

```text
Admin Account
```

支持：

```text
username
password
```

Password hash：

```text
Argon2id
```

---

# 88. Session

要求：

```text
HttpOnly
Secure
SameSite=Lax/Strict
```

禁止把 Manager session 存 localStorage。

---

# 89. Docker 权限

Provisioner 可以访问：

```text
/var/run/docker.sock
```

Web frontend 不允许。

最好：

```text
Manager Web
↓
Provisioner internal API
↓
Docker Socket
```

---

# 90. Provisioner API

只允许：

```text
localhost
```

或：

```text
private Docker network
```

不得公网暴露。

---

# 91. API

主要：

```text
GET    /api/projects

POST   /api/projects

GET    /api/projects/:id

PATCH  /api/projects/:id

DELETE /api/projects/:id
```

---

# 92. Lifecycle API

```text
POST /api/projects/:id/start

POST /api/projects/:id/stop

POST /api/projects/:id/restart
```

---

# 93. Service API

```text
POST /api/projects/:id/services/:service/start

POST /api/projects/:id/services/:service/stop

POST /api/projects/:id/services/:service/restart
```

---

# 94. Config API

```text
GET
/api/projects/:id/config

PATCH
/api/projects/:id/config
```

---

# 95. Auth API

```text
GET
/api/projects/:id/auth

PATCH
/api/projects/:id/auth
```

Provider：

```text
PATCH
/api/projects/:id/auth/providers/google
```

---

# 96. Storage API

```text
PATCH
/api/projects/:id/storage
```

---

# 97. Functions API

```text
PATCH
/api/projects/:id/functions/config
```

---

# 98. Operation API

```text
GET
/api/operations/:id
```

用于进度条。

---

# 99. 前端信息架构

```text
Projects

Project
├── Overview
├── Services
├── Authentication
│   ├── General
│   ├── Providers
│   ├── URL
│   └── SMTP
│
├── Database
├── Storage
├── Realtime
├── Functions
├── Connection Pool
├── Logs
├── Network
├── Secrets
├── Backups
└── Settings
```

---

# 100. UI 风格

建议参考：

```text
Supabase Studio
```

视觉语言：

- 深色/浅色
- 紧凑 Dashboard
- 左侧导航
- Card
- Toggle
- Code field
- Status Badge
- Monospace secrets

但不复制 Supabase 商标和受保护品牌资产。

---

# 101. Installation Preset UX

首页：

```text
New Project

Choose configuration

● Lightweight
  DB + Auth + REST + Studio

○ Standard
  + Realtime + Storage + Functions + Pooler

○ Full
  All official services

○ Custom
```

推荐：

```text
Lightweight
```

---

# 102. Dependency Engine

这是 Manager 核心之一。

规则示例：

```text
Studio
→ requires postgres-meta

imgproxy
→ requires Storage

Vector
→ requires Logflare

Logs
→ requires Vector + Logflare

All APIs
→ require Gateway

Auth / REST / Realtime / Storage
→ require Database
```

用户操作时自动判断。

---

# 103. Invalid Config Prevention

例如用户：

```text
Storage OFF
imgproxy ON
```

UI：

```text
Image Transformation requires Storage.
```

并禁止保存。

---

# 104. Resource Awareness

Dashboard 显示：

```text
Project CPU

Project RAM

Project Disk
```

以及全机：

```text
Host

CPU 31%

RAM 6.2 / 16GB

Disk 84 / 200GB
```

---

# 105. Resource Warning

创建项目时如果服务器：

```text
RAM available < threshold
```

提示：

```text
Server resources may be insufficient
for the selected configuration.
```

但不做复杂“成本预测”。

---

# 106. V1 默认行为

用户只输入：

```text
Name

Domain

Site URL
```

其余全部保持默认：

```text
DB ON
Gateway ON
Auth ON
REST ON
Studio ON
postgres-meta ON

Realtime OFF
Storage OFF
imgproxy OFF
Functions OFF
Supavisor OFF
Logs OFF
```

然后可以立即安装。

---

# 107. V1 必须成功的核心流程

## Flow A

```text
Create Bee
↓
Lightweight
↓
Install
↓
Studio available
↓
Auth works
↓
REST works
```

---

## Flow B

```text
Authentication
↓
Google
↓
Enable
↓
Client ID + Secret
↓
Save
↓
Auth recreate
↓
/auth/v1/settings
↓
google:true
```

---

## Flow C

```text
Storage
↓
Enable
↓
R2
↓
Credentials
↓
Save
↓
Storage starts
↓
Upload test
```

---

## Flow D

```text
Functions
↓
Enable
↓
Save
↓
Edge Runtime starts
↓
hello function works
```

---

## Flow E

```text
Realtime
OFF → ON
↓
service starts
↓
WebSocket available
```

---

# 108. 验收标准

## 多项目

至少同时运行：

```text
Bee
Nomo
Project C
```

三套独立 Supabase。

---

## 隔离

必须满足：

```text
DB A ≠ DB B

Auth users A ≠ B

JWT A ≠ B

Service Role A ≠ B

Storage A ≠ B

Functions secrets A ≠ B
```

---

## 服务管理

所有官方可选服务均能：

```text
UI Enable

UI Disable

Configure

Restart

Health Check
```

---

## OAuth

至少验证：

```text
Google
GitHub
Apple
```

并确保 Provider 只影响对应 Project。

---

## Storage

必须验证：

```text
Local

S3 Compatible / R2
```

---

## Functions

必须验证：

```text
Function execution

Function secrets

Restart

Recreate
```

---

## Logs

开启后：

```text
Logflare
Vector
Studio Log Explorer
```

工作正常。

---

# 109. 技术红线

禁止：

### 1

修改 Supabase PostgreSQL initialization SQL。

### 2

自行复制 Supabase role / schema 创建逻辑。

### 3

让多个 Project 共用 JWT Secret。

### 4

让多个 Project 共用 Auth database。

### 5

在 Web frontend 直接调用 Docker Socket。

### 6

将 OAuth Secret / Database Password 明文存 Manager Database。

### 7

默认启用全部服务。

### 8

使用 Docker image `latest` 作为生产默认版本。

---

# 110. 开发阶段

## Phase 1 — Installer Core

实现：

- Manager Login
- Project Model
- Docker Provisioner
- Secrets Generator
- Official Supabase Template
- Lightweight install
- Start / Stop / Delete
- Health

---

## Phase 2 — Complete Service Manager

加入：

- Realtime
- Storage
- imgproxy
- Functions
- Supavisor
- Logs
- Vector
- Gateway options

---

## Phase 3 — Auth Manager

加入：

- Email
- Phone
- Anonymous
- URLs
- SMTP
- 全部 OAuth Providers
- Provider verification
- Rollback

---

## Phase 4 — Storage & Functions

加入：

- Local
- S3
- R2
- S3 Protocol
- Functions secrets
- Function runtime controls

---

## Phase 5 — Operations

加入：

- Logs
- Metrics
- Version updates
- Backup
- Restore
- Config import/export

---

# 111. Definition of Done

V1 完成必须满足：

- [ ] 使用官方 Supabase Docker Runtime
- [ ] 一个 Project 一套独立 Runtime
- [ ] Web 创建多个 Project
- [ ] PostgreSQL 可配置
- [ ] Envoy Gateway 可配置
- [ ] Auth 可配置
- [ ] PostgREST 可配置
- [ ] Studio 可配置
- [ ] postgres-meta 可配置
- [ ] Realtime 可开关
- [ ] Storage 可开关
- [ ] Local Storage 可配置
- [ ] S3 Storage 可配置
- [ ] R2 可配置
- [ ] S3 Protocol 可配置
- [ ] imgproxy 可开关
- [ ] Edge Functions 可开关
- [ ] Functions secrets 可配置
- [ ] Supavisor 可配置
- [ ] Logs / Logflare 可开关
- [ ] Vector 可开关
- [ ] Google OAuth 可视化配置
- [ ] GitHub OAuth 可视化配置
- [ ] Apple OAuth 可视化配置
- [ ] 官方 OAuth Provider 列表均有配置入口
- [ ] Site URL 可配置
- [ ] Redirect URLs 可配置
- [ ] SMTP 可配置
- [ ] API Keys 自动生成
- [ ] Secrets 加密
- [ ] Domain 可配置
- [ ] HTTPS / Reverse Proxy 有明确模式
- [ ] 端口自动分配
- [ ] Project Start/Stop/Restart/Delete
- [ ] Service Restart/Recreate
- [ ] Health Check
- [ ] Operation Progress
- [ ] Failure Rollback
- [ ] Docker Logs
- [ ] Version Pinning
- [ ] 一个项目配置改变不会影响其他项目
- [ ] 不修改 Supabase 官方数据库初始化逻辑

---

# 112. 最终产品形态

最终产品不是：

```text
Supabase fork
```

也不是：

```text
Supabase-compatible backend
```

而是：

```text
Official Supabase Runtime
          +
Visual Installer
          +
Multi-instance Manager
          +
Service Configuration
          +
Auth Provider Configuration
          +
Docker Orchestration
```

最终用户体验：

```text
Supabase Manager

Projects

Bee                Healthy
Nomo               Healthy
Project X          Stopped

+ New Project
```

进入 Bee：

```text
Overview

Services

Authentication

Database

Storage

Realtime

Functions

Connection Pool

Logs

Network

Secrets

Backup

Settings
```

所有底层仍然是：

> **官方 Supabase。**

Manager 只负责让原本需要 SSH + `.env` + Compose 操作的 Self-hosting 流程，变成一个稳定、可重复、可回滚的 Web 产品。

---

# 113. 最核心的设计原则

**简单项目应该在 3 个字段后即可安装：**

```text
Project Name
Domain
Site URL
```

所有非关键服务默认关闭。

高级用户再逐步开启：

```text
Realtime
Storage
Functions
Supavisor
Logs
OAuth
SMTP
S3
R2
```

因此本产品同时满足：

```text
简单项目
→ 极轻量

复杂项目
→ 完整 Supabase Stack
```

而无需维护两套部署系统。