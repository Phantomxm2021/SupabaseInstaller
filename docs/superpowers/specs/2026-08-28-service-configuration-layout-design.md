# 创建项目向导与服务配置重排设计

## 已确认方向

采用 **A：四步渐进式向导**。它以“默认快速创建、按需打开高级设置”为原则，保留当前全部配置能力但不强迫每个用户逐项填写。

服务选择采用 **预设侧栏 + 服务分组**：预设始终在左侧可见，当前预设的服务按职责在右侧显示。任何服务修改都会把预设标记为 Custom。

## 向导结构

1. **项目基础**：项目名称、项目标识符、访问域名、Studio 管理员用户名和密码。这些字段均为单列排布；管理员用户名不能收进高级设置。
2. **服务组合**：Lightweight、Standard、Full、Custom 预设，加上按“核心服务”和“扩展能力”划分的服务开关。
3. **安全与集成**：Authentication、SMTP、对象存储与图片转换、Edge Functions。每个模块以折叠标题呈现，启用开关放在标题最右侧。
4. **基础设施与审核**：数据库、Realtime、Supavisor、Gateway、HTTPS、分配端口及最终摘要。该步骤提供返回到对应模块修改的入口。

步骤切换使用 180–220ms 的方向性过渡：前进时当前内容向左淡出、下一步从右侧滑入；返回时方向反转。校验失败时停留当前步骤、轻微提示错误并将焦点移动到首个错误字段。

## 项目基础与校验

- 项目名称输入时进行格式校验；项目名称和项目标识符在停止输入约 400ms 后分别进行异步重复检查。
- 只有同步校验通过、重复检查完成且没有冲突时，Continue 才可用。
- 网络检查失败显示可恢复的“暂时无法验证”状态，并提供重试，不把失败误判为重复。
- 项目标识符默认由项目名称生成，但可手动修改；它用于项目 URL 和目录名。

## 服务组合

- 左侧显示 Lightweight、Standard、Full、Custom 及其简短说明；当前项使用深绿色填充与绿色描边。
- 右侧显示当前预设的服务、启用数量及分组开关。
- 必需项与依赖项使用低调标签说明：PostgreSQL 必需；Studio 依赖 postgres-meta；Image Transformation 依赖 Storage；Logs 和 Vector 必须一起启用；服务 API 依赖 API Gateway。
- 对可编辑服务切换后，选中预设变为 Custom，并显示内联提示。

## 安全与集成

安全与集成页不展示“邮箱登录已启用”等顶部状态行。模块默认折叠，开启后再展示模块字段；模块开关统一在折叠标题最右侧。

Authentication 内部使用“添加登录方式或 OAuth Provider”操作，而不是一次性列出全部认证表单。该操作打开带搜索框的单列弹窗列表，支持按名称搜索并以“全部 / 登录方式 / OAuth Provider”筛选。

- 可添加登录方式：Email password、Magic Link、Phone Auth、Anonymous sign-in；可添加的邮件投递配置为 Custom SMTP。
- OAuth Provider 列出所有当前受支持的 Provider，包括 Apple、Azure、Bitbucket、Discord、Facebook、Figma、GitHub、GitLab、Google、Kakao、Keycloak、LinkedIn、Notion、Slack、Snapchat、Spotify、Twitch、Twitter、WorkOS、Zoom。
- 添加 OAuth Provider 即表示启用，并创建所需 Client ID、Secret 和 Provider 特有字段。已添加 Provider 不可重复添加。
- Provider 卡片右上角只有垃圾桶图标移除按钮；移除时二次确认，确认后删除该 Provider 的配置。
- Authentication 总开关关闭时保留已配置 Provider 草稿，但项目安装时不启用 Authentication。
- Phone Auth 仅在添加后显示供应商和条件字段；Custom SMTP 仅在模块开启后要求 Host、Port、用户名、密码、发件人邮箱与名称。

Storage 启用后展示后端（本地、AWS S3、Cloudflare R2、兼容 S3）、Bucket、Region、Endpoint 或 Account ID、Access Key、Secret 和 Force path style。Image Transformation 依赖 Storage。Edge Functions 启用后显示函数目录、默认 JWT 验证和秘密环境变量。

## 基础设施与审核

- 高级基础设施区包含 PostgreSQL 版本、最大连接数、Shared buffers、Realtime 参数、Supavisor 连接池、Gateway、HTTPS mode 及 Manager 分配的只读端口。
- 选择 Caddy 时自动启用 API Gateway；外部反向代理模式说明 TLS 在 Manager 之外终止。
- 最终审核页不显示秘密明文；缺少的必填凭据标记为“需要完成”。每条摘要都能返回所属步骤进行编辑。

## 视觉与响应式规则

- 保留当前深色主题、低对比描边、浅色正文和绿色主强调色。
- 服务配置采用左侧预设、右侧服务分组；项目基础和认证添加弹窗使用单列阅读流。
- 窄屏下侧栏与双栏分组依次堆叠，认证添加列表始终保持单列。
- 交互控件保留足够可点击面积，并为异步校验、加载、错误与成功状态提供文本提示和 `aria-live` 宣告。

## 验收标准

- 常规用户可仅填写五项基础信息、选择预设、确认默认安全配置后完成安装。
- 高级用户可在对应步骤展开并配置当前已有的全部服务、集成和基础设施字段。
- 用户能找到并理解服务依赖、校验状态、认证 Provider 的添加与移除方式。
- 不改变后端创建请求、依赖约束、秘密写入语义或 Manager 的端口分配行为。
