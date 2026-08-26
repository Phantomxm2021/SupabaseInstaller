# Task 7 Review — CHANGES REQUIRED

审计范围：`7d53e5f..2feb646`。我将 Task 7 plan/design、`internal/contracts/configuration.go`、Manager defaults/validation、Provisioner provider registry/renderer与本提交逐字段对照；没有把 `task-7-report.md` 或既有测试通过当成验收证据。没有发现 Critical；存在多个 Important，因此本轮不予 `APPROVED`。

## Important

- **Important — `apps/web/src/features/projects/NewProjectPage.tsx:26`：最终 POST 完全绕过 React Hook Form resolver，且 Basic 的“Review”连四个基础字段都不校验。** “Review”直接 `setStep(5)`，安装按钮直接 `create.mutate(form.getValues())`，没有任何 `handleSubmit`/`trigger`；常规路径也仅在 step 4 的 Continue 上全量 `trigger()`，用户到过 Review 后可返回任一步修改为非法值，再点击已解锁的 Review tab 绕过校验。结果是空 name/domain/site URL、非法 SMTP/OAuth/Storage/Functions/port 组合均会发送到 API，错误只能以顶层 mutation message 显示，不能附着到字段。即便走 step 4，除 Basic 外的组件也不渲染 `formState.errors`，所以 Zod 拒绝时页面会原地无反馈。提交必须从一个真实 `<form onSubmit={handleSubmit(...)}` 进入，并按 step 显示对应 field errors。

- **Important — `apps/web/src/features/projects/AuthStep.tsx:8`、`StorageFunctionsStep.tsx:6`、`DatabaseNetworkStep.tsx:6`：所谓六步“完整配置”没有提供大量 Go aggregate 字段，用户无法创建设计要求的 Custom 项目。** Auth 缺少 `secureEmailChange`、`doubleConfirmChanges` 以及完整 Phone Auth provider/secret/provider fields；Realtime 的三个字段全部没有表单；Storage 缺少 `forcePathStyle`；Database 缺少 `sharedBuffers`/`extensions`；Pooler 缺少 `maxClientConnections`；Network 缺少 internal/API/Studio/direct DB/pooler port 展示或输入及 certificate；Review 前也没有任何替代入口。默认对象让 POST 在结构上带齐这些字段，但固定默认值不等于“typed configuration experience”，尤其违背 Task 7 明确要求创建时可配置全部 Auth、Realtime、Storage、Functions、DB、pooler 和 network 设置。

- **Important — `apps/web/src/features/projects/NewProjectPage.tsx:26`：新增 wizard 没有使用任何计划指定的 shadcn 表单/布局 primitive。** 六个步骤全部由原生 `<input>/<select>/<textarea>/<button>` 和 legacy `.panel`/新增手写 CSS 组成；新增项目/operation 文件没有一次 `components/ui/*` import，也没有 `Tabs`、`Card`、`Field/Form`、`Switch`、`Select`、`Collapsible`、`Input` 或 `Alert` composition。这不是“actual generated shadcn components and composition patterns”，直接违反 design §7 和 Task 7 Step 4，而不仅是视觉细节。

- **Important — `apps/web/src/features/projects/StorageFunctionsStep.tsx:6` 与 `DatabaseNetworkStep.tsx:6`：这些页面内的手动服务修改既不把 preset 设为 `CUSTOM`，也没有同步重复的 authoritative flags。** 只有 `PresetStep.toggle()` 会设 Custom；Storage、imgproxy、Functions、Supavisor、`database.directPort`、`auth.enabled` 的页面内修改都保留原 preset 标签。更严重的是 Auth step 改的是 `configuration.auth.enabled`，实际 Compose 选择读 `configuration.services.auth`；DB step 改的是 `database.directPort`，而 Manager 端口分配读 `services.directDb`；两者都可能与 Preset step 的服务开关矛盾。比如 Lightweight 中勾选 Direct PostgreSQL port 仍保留 `services.directDb=false` 和 allocated network port 0，Provisioner 会在 `apps/provisioner/internal/render/render.go:66-79` 拒绝。所有服务入口应走同一 dependency-closure/action，并在手动变化时原子更新 Custom 与关联字段。

- **Important — `apps/web/src/features/projects/StorageFunctionsStep.tsx:6`：Storage backend 切换会保留随后不可见的旧字段，且 S3-compatible API 并不独立。** 从 generic/AWS S3 填入 endpoint/credentials 后切回 Local，组件只改 `backend` 并隐藏清理控件，Zod/Go 却拒绝 Local 携带这些字段；从 generic S3 切到 R2同样会因隐藏的 endpoint 被拒绝。用户没有办法从当前 UI 恢复合法状态。与此同时 `s3CompatibleApi` 开关只在 object-storage 分支渲染，无法在 Local backend 独立选择，违反 design §3.6 的“backend choice and Enable S3-compatible API are independent settings”。切换 backend 必须正规化互斥字段，并始终暴露独立 protocol setting。

- **Important — `apps/web/src/features/projects/OAuthProviderFields.tsx:9`：首次启用任意 OAuth provider 时只写入 `.enabled`，没有把展示用 fallback 对象写回 form。** `oauth` 初始是 `{}`；勾选后 RHF 中的 entry 缺少 schema 强制的 `secretSet`（且 fields 只靠后续 register 偶然生成）。用户即使填写 client ID 和 client secret，step 4 的全量 `trigger()` 仍会在不可见的 `secretSet` 上失败，且 provider card 不显示错误。快速 Review 之所以可能让相同输入到达服务器，只是因为上述提交绕过了 Zod。启用时应原子初始化完整 provider DTO，并逐字段展示 conditional errors。

- **Important — `apps/web/src/features/projects/projectSchema.ts:12-20`：Zod 并不是 Go validation 的完整镜像，多个服务端拒绝条件在客户端可通过。** `secretSchema` 不要求 `replace` 携带非空 value；OAuth record 不拒绝未知 provider，且任一特殊 provider 都可带四种特殊字段中的任一种，而 Go 要求 provider/field 精确配对；enabled OAuth 没有拒绝 `remove` 的完整状态表；Phone Auth 完全没有 provider、required fields、secret action 校验；`disableSignup=true` 没有拒绝 Phone/anonymous/OAuth；Local Storage 没有检查 `secretAccessKey.action`，object storage 没有完整 retain/replace/remove 约束；OAuth/Storage URL 使用通用 `z.url()` 而非 Go 的 http/https；domain regex 接受单标签、空 label 和非法 DNS label；端口没有唯一性/关联约束。这里需要按 Manager field paths 与 pinned renderer constraints 做表驱动 parity，而不是若干局部 refine。

- **Important — `apps/web/src/features/projects/DatabaseNetworkStep.tsx:6`：UI 提供一个必然安装失败的 Manual certificate 选项，却没有 certificate 字段。** 选择 `manual` 可通过当前 Zod（`certificate` 允许空字符串）并进入创建；Provisioner 在 `apps/provisioner/internal/render/render.go:57-59` 对所有 manual HTTPS 直接报错。要么 Task 7 不展示尚未支持的模式，要么按设计提供完整 typed certificate flow 并让 renderer 支持；当前行为会创建一个注定 FAILED 的 project/operation。

- **Important — `apps/web/src/features/projects/ReviewStep.tsx:5`：Review 不是设计要求的完整 redacted aggregate summary，并会把“enabled”误报为“Configured”。** 页面没有 Auth enablement/email/signup/session/redirect/phone 摘要，也没有 database、pooler、Realtime 或 network 摘要；SMTP 只要 `enabled` 就显示 “Configured”，Functions 只要 service 开启也显示 “Configured”，不检查 required secret action/variable validity。通过快速 Review 时，空 SMTP password 也会被呈现为 Configured。实现确实没有直接输出 secret value，但 redaction 不等于省略绝大部分配置或谎报配置完成状态。

- **Important — `apps/web/src/api/types.ts:22-39` 与 `apps/web/src/api/types.ts:41-52`：TypeScript JSON contract 不是 Go contract 的 exact mirror。** Go `SecretInput.Action` 没有 `omitempty`，正常 redacted response 会编码 `action:""`，但 TS 只允许三种非空 action 或属性缺失；多个 Go `omitempty` collection/string 在 TS 中却被声明为必填。更直接的是 Go `Project` 返回 `configurationRevision` 且不返回 `configuration`，TS `Project` 恰好漏掉前者并虚构了后者。类型必须忠实描述实际 wire DTO（必要时区分 create input、redacted projection 与 Project list DTO），否则后续 Task 8 会在静态类型已撒谎的基础上工作。

- **Important — `task-7-report.md:9`：报告声称的 TDD/coverage 与提交内容不符，Task 7 没有任何测试变更。** `git diff --name-only 7d53e5f..2feb646` 中没有 test/spec 文件。既有 `NewProjectPage.test.tsx:7-29` 只走绕过校验的 Lightweight 快速 Review，只断言顶层 preset 与 Installing heading，未断言 `configuration`；既有 `OperationPanel.test.tsx:5-13` 只测 FAILED 文案/按钮，未挂 Router、未测 projectId、SUCCEEDED once、query invalidation、replace history 或任一 terminal failure 不导航。因此 17 个既有 tests 通过无法证明 report 所称 Standard closure、Custom、SMTP、OAuth callback、完整 payload 或 success navigation，且违反 Task 7 Step 1/2/7 的明确交付要求。

## Minor

- **Minor — `apps/web/src/features/operations/OperationPanel.tsx:20`：`useNavigate()` 被条件调用，违反 React Hooks 的固定调用顺序规则。** Router context 在当前挂载中通常稳定，所以现有无 Router 用例未立即崩溃；但 hook 不应位于 ternary 分支。应把导航职责收敛到始终处于 Router 的 wrapper/callback，或提供两个固定 hook topology 的组件。

- **Minor — `apps/web/src/features/operations/OperationPanel.tsx:41-43`：成功副作用没有 await projects invalidation，且同时暴露 callback和内建 navigation 两个导航入口。** 当前 New Project 传入 no-op，所以 direct replace navigation 由 `handledSuccess` 对 active operation ID 去重，FAILED/ROLLED_BACK/CANCELLED 也确实不导航；但真实 `onSucceeded` 若也导航会产生两次路由动作，且 invalidate Promise 的完成顺序没有定义。建议只保留一个 success owner，显式 `await invalidateQueries(['projects'])` 后再 replace-navigate，并用真实 history/query timeline 测试锁定 exactly once。

## Verified

- Go `DefaultConfiguration` 与前端 `defaultConfiguration` 的当前 scalar defaults、Lightweight/Standard/Full/Custom service sets相符；20 个 OAuth provider 名称和四个特殊字段名称也与 contracts/provider registry 相符。
- POST body 当前确实包含 top-level legacy projection及完整 `configuration` aggregate；旧的固定 `lightweightServices` POST 路径已被移除，未发现第二个固定 Lightweight renderer。
- Review 没有把 SMTP/OAuth/Storage/Functions secret plaintext直接渲染出来；Functions textarea值也没有出现在 Review JSX。
- `OperationPanel` 当前 direct success path以 active operation ID 去重，使用 operation/create response project ID，invalidate `['projects']` 并 replace 到 `/projects/:id/overview`；FAILED、ROLLED_BACK、CANCELLED 不进入该 effect。
- 独立运行 `npm run test --workspace apps/web -- --run`：PASS，9 files / 17 tests；这些是提交前已存在的 tests，覆盖缺口见上。
- 独立运行 `npm run build --workspace apps/web`：PASS；Vite仅报告 689.41 kB chunk warning。
- `git diff --check 7d53e5f..2feb646`：PASS。

---

# Task 7 Fix Round 1 Review — CHANGES REQUIRED

复审范围：`2feb646..5bcb974`。逐项回归上一轮 11 个 Important 与 2 个 Minor，并重新对照 Go creation secret semantics、Manager validation、Provisioner renderer 和真实新增 tests。发现 1 个 Critical，仍有 Important，因此本轮不予 `APPROVED`。

## Critical

- **Critical — `apps/web/src/features/projects/projectSchema.ts:36-37`：新的默认 secret marker 使所有默认项目创建稳定返回 422，Lightweight 主路径完全不可用。** `defaultConfiguration()` 现在把 SMTP、Phone 与 Local Storage secret 都初始化为 `{action:'retain'}`，而前端 schema 将它判为合法；POST 会原样包含这些 marker。Manager 对 Local Storage 明确要求 `secretAccessKey.action == ""`（`apps/manager/internal/project/configuration.go:341-345`），所以 `ValidateDraft` 已会拒绝默认 aggregate；即使越过该处，initial create 的 `encryptConfigurationSecrets` 也明确对任何 `retain` 返回 “cannot retain a secret during project creation”（`apps/manager/internal/project/service.go:79-87`），首先会在 disabled SMTP 上失败。独立执行 `projectConfigurationSchema.safeParse(defaultConfiguration('LIGHTWEIGHT'))` 得到 success，证明客户端错误地放行这个服务端必拒请求。`NewProjectPage.test.tsx` 的 mocked fetch 永远返回 202，因而把完全不可创建的产品路径报告成通过。Create input 必须使用 create-safe empty/remove/replace semantics，不能复用 update/redacted 的 retain marker，并应增加真实 Manager create contract test。

## Important

- **Important — `apps/web/src/components/ui/form.tsx:1-6` 与 `apps/web/src/components/ui/alert.tsx:1-6`：新增同名组件只是给原生元素套 class 的手写伪 primitive，不是 generated shadcn composition。** `Form` 没有 RHF context/control/field composition，仅返回 `<form>`；`Alert` 也没有生成组件的 variants、Title/Description contract。Wizard footer/preset actions仍使用 legacy `.button`/原生 button。Card、Field、Input、Select、Switch、Tabs 等既有 generated primitives 的采用是真实进展，但创建两个六行同名 wrapper 不能满足 design 的“actual generated shadcn components rather than visually imitating them”，上一轮 shadcn Important 只关闭了一部分。

- **Important — `apps/web/src/features/projects/AuthStep.tsx:14`、`AuthStep.tsx:28-30` 与 `PresetStep.tsx:29`：RHF 现在会阻止非法跳转/submit，但仍没有把大量 schema error 渲染到对应字段，用户会无反馈地卡住。** Auth 的通用 `TextField`/`NumberField` 根本不读取 `formState.errors`，因此 SMTP host/username/sender、Phone account/message service/originator/sender 等 required errors不可见；Functions 只查询数组根 `configuration.functions.variables`，而 Zod 报在 `[i].name`/`[i].value.value`，也不会显示；Preset service switches 没有任何 `FieldError`，关闭 required gateway、meta 或制造 dependency conflict 后 Continue/Review 只是不动作。真正的 form/trigger/handleSubmit boundary已修复，但 report 所称“field errors render next to corresponding typed control”不成立。

- **Important — `apps/web/src/features/projects/PresetStep.tsx:26`：选择 preset 仍只替换 services，不能恢复与服务重复的 typed flags，重新制造 service/config drift。** 例如用户从 Auth step 关闭 Auth 后，helper 同时写入 `services.auth=false` 与 `auth.enabled=false`；随后选择 Lightweight/Standard/Full，`setPreset` 只把 `services.auth` 改回 true，`auth.enabled` 仍 false。Review 会说 Auth Disabled，但 Provisioner Compose selection仍启动 Auth。Preset application应原子应用 Go-compatible aggregate/default closure或至少同步所有 duplicated authoritative fields，而不只是 services slice。

- **Important — `apps/web/src/features/projects/PresetStep.tsx:18` 与 `DatabaseNetworkStep.tsx:16-19`：Direct DB 和 allocated network controls绕开 Manager allocation，并暴露多个 renderer 必拒输入。** 勾选 Direct DB 会硬编码 `54322` 到 database/network 两个端口，而不是保留 0 让每个项目由 allocator分配；第二个项目或宿主占用该端口时会在持久化/安装前冲突。页面两处可见的 direct-port NumberField又都绑定 `network.directDatabasePort`，没有实际渲染 `database.directPortNumber`。API、Studio、internal gateway等自动分配端口也被做成普通可编辑 Input，违反 design 的 read-only allocator约束。更直接的是 Extensions 输入允许任意非空列表，而 pinned renderer 在 `apps/provisioner/internal/render/render.go:63-65` 拒绝所有 extensions；internal gateway schema/UI允许任意 1–65535，但 renderer仅接受 0 或 8000（第 60–61 行）。这些不是字段“渲染出来”就算完成，而是可稳定创建 FAILED operation 的无效 controls。

- **Important — `apps/web/src/features/projects/AuthStep.tsx:21` 与 `projectSchema.ts:19`：Phone form仍缺少官方支持的 Twilio `verifySid`，且 disabled Phone 的 Zod truth table与 Go相反。** Go/provider renderer允许 `accountSid`、`messageServiceSid`、`verifySid`，UI只渲染前两项。Zod 又在 `phone.enabled` 为 false 时照样要求所选 provider 的 required fields；Provider Select始终可操作但这些 fields仅在 enabled 时渲染，用户先选 Twilio或关闭已配置 Phone后可被隐藏错误阻塞。Go `validatePhone` 只在 enabled 时要求 provider fields。这里仍未做到实际字段完整性和 Go parity。

- **Important — `apps/web/src/features/projects/projectSchema.ts:15-34`：Zod 与 Manager/renderer constraints仍有可复现差异。** 除上述 secret/Phone外：domain regex继续接受 Go拒绝的普通单标签 hostname；`SecretInput.action==""` 是 Go redacted/default合法状态，却被 Zod enum拒绝；Functions-only/Caddy/Storage dependency closure没有完整包含 Provisioner的 gateway/REST要求；任意 nonempty extensions和 internal gateway 1234 都可通过；service relation没有约束 `services.auth == auth.enabled`。独立 schema探针得到：single-label domain、extensions、internalGatewayPort 1234、Functions enabled without gateway均 accepted，而 Go/renderer会拒绝或产生语义漂移。应从 Go field-path table和 pinned renderer admission规则生成对称 cases，不能只靠当前手写 refinements。

- **Important — `apps/web/src/api/types.ts:22-39` 与 `projectSchema.ts:37`：wire DTO separation和 defaults parity仍是名义上的。** `RedactedProjectConfiguration` 与 `CreateProjectConfiguration` 是完全相同的结构，前者仍允许 `SecretInput.value`，后者反而要求每个 secret都有 action；Phone provider/fields、redirectUrls、OAuth、Functions variables、Database extensions等 Go `omitempty` 成员仍在 redacted TS type中必填。前端 Local `localPath` 还从 Go `DefaultConfiguration` 的空字符串擅自改成 `./volumes/storage`。`configurationRevision`/不存在的 `Project.configuration` 已修复，`action:""` 也加入 union，但 create input、redacted projection、update marker和 Go defaults必须真正分开建模，不能只换三个 interface 名称。当前错误抽象正是 Critical default retain回归的直接原因。

- **Important — `apps/web/src/features/projects/NewProjectPage.test.tsx:44-60` 与 `OperationPanel.test.tsx:5-13`：新增 tests仍没有覆盖计划要求的行为，且 success navigation完全没有新增测试。** “complete aggregate”用全默认值连点 Continue，只断言五个已知属性，未操作 Standard/Full/Custom、任何 shadcn Select/Switch、SMTP/OAuth/Phone/Storage/Functions、normalization、secret action或 nested field error；mock 202也无法发现默认 payload被真实 Go拒绝。OperationPanel suite仍只有原来的 FAILED 文案/按钮用例，没有 Router、SUCCEEDED、retained projectId、awaited `['projects']` invalidation、replace history、exactly-once callback，亦未分别证明 FAILED/ROLLED_BACK/CANCELLED不导航。Task 7 Step 1与上一轮 test/report finding仍未关闭。

## Minor

- **Minor — `apps/web/src/features/projects/ReviewStep.tsx:13-14`：Functions的合法零变量配置被显示为“No configured secrets”。** Edge Functions不要求至少一个自定义 secret；默认 JWT设置加零变量是完整合法配置。Review现在覆盖 Auth、Email、Phone、SMTP、OAuth、Storage、Realtime、DB、Pooler和Network且不输出 plaintext，上一轮主体 finding已关闭，但这里应显示“0 variables”或“Configured”，而不是暗示缺失配置。

- **Minor — `task-7-report.md:51`：Round 1报告记录的 commit hash是 `70a1205`，审计范围中的实际提交是 `5bcb974`。** 若是 rebase/cherry-pick后的 hash，应更新交付记录，避免后续复审无法按报告定位对象。

## Round 1 findings resolved

- Review shortcut、Tabs forward navigation和 Install现在都进入 RHF validation；Install使用真实 form submit，Basic required errors可见，上一轮“完全绕过 resolver”实现问题已关闭（但 nested errors仍见 Important）。
- 实际 Card/Field/Input/Select/Switch/Tabs/Collapsible/Textarea等 generated components已在六步中使用，并补出了绝大部分 Auth/Phone/SMTP/OAuth、Storage、Realtime、Functions、DB、Pooler、Network字段；伪 Form/Alert与少量遗漏见上。
- `setServiceEnabled`成为跨步骤服务 mutation入口，Storage/Functions/Supavisor/Auth/DirectDB会设 Custom并同步主要 service flags；preset重新应用的 drift仍见上。
- Storage backend切换现在清理互斥字段，S3-compatible API始终独立显示；OAuth enable会写回完整 entry，上一轮两项实现缺陷已关闭（creation secret marker引入新的 Critical）。
- Manual certificate不再出现在 schema或 Select中，避免了上一轮必然 renderer失败的 manual路径。
- Review已经按类别提供 redacted summary，SMTP/OAuth/Storage secret状态不再只按 enabled误报，未发现 plaintext输出；仅保留零 Functions变量文案 Minor。
- `Project.configurationRevision`已加入且虚构的 `Project.configuration`已删除；SecretAction已包含空字符串。DTO separation/omitempty仍未完成。
- OperationPanel通过 Routed/Core拆分固定 hook topology；成功时 await projects invalidation，并在 callback与内建 navigate之间只选一个 owner；按代码审查 SUCCEEDED用 active operation ID去重，FAILED/ROLLED_BACK/CANCELLED不导航。上一轮两个 OperationPanel实现 finding已关闭，但没有 success regression tests。
- 旧 fixed Lightweight POST路径仍不存在；POST继续包含 legacy projection和完整 aggregate。

## Round 1 verification

- 独立运行 `npm run test --workspace apps/web -- --run`：PASS，9 files / 19 tests；Critical未被 mock-only suite捕获。
- 独立运行 `npm run build --workspace apps/web`：PASS；Vite报告 768.27 kB chunk warning。
- `git diff --check 2feb646..5bcb974`：PASS。
- 独立 Node schema探针：默认 `retain` aggregate被前端接受；disabled Twilio产生隐藏 required-field errors；extensions、internal gateway 1234、Functions without gateway、single-label domain均被前端接受；Go empty secret action反被前端拒绝。

---

# Task 7 Fix Round 2 Review — CHANGES REQUIRED

复审范围：`5bcb974..9f1bea0`。本轮逐项回归 Round 1 的 1 个 Critical、7 个 Important 与 2 个 Minor，并以 `9f1bea0` 的独立快照对照 Manager create/secret 逻辑和 Provisioner admission；没有把 `task-7-report.md` 或 tests 名称当成验收证据。上一轮 Critical 已关闭，但仍有 5 个 Important，因此本轮不予 `APPROVED`。

## Important

- **Important — `apps/web/src/features/projects/PresetStep.tsx:19`、`projectSchema.ts:33` 与 `DatabaseNetworkStep.tsx:16-19`：Direct DB 开关仍会创建一个 Zod/Manager 接受、Provisioner 必然拒绝的 aggregate，“Manager allocates”并不真实。** `setServiceEnabled(..., 'directDb', true)` 把 `services.directDb` 设为 true，却故意把 `database.directPort`、`database.directPortNumber` 和 `network.directDatabasePort` 全部置 0；第 33 行只拒绝“服务关闭但端口非零”，没有要求启用时存在端口，所以可以 POST。实际安装器只在 `apps/manager/internal/install/orchestrator.go:60-68` reserve API port，没有 reserve Direct DB/Studio/pooler；Provisioner `apps/provisioner/internal/render/render.go:66-70` 则明确要求 Direct DB 启用时两个候选端口至少一个有效，因此该 UI 开关会稳定进入 FAILED。相同所有权问题也存在于页面/Review 的 “Allocated by Manager”：Standard/Full 的 Supavisor仍使用固定 `6543/6544`，初始 create transaction并不 reserve/check这些端口，第二个项目或宿主占用时只能到 Compose阶段失败。应先让 Manager真正分配并持久化所有 server-owned ports，或在 create前提交一个 renderer可用且冲突受控的端口请求；不能只把控件改成 0/read-only。

- **Important — `apps/web/src/features/projects/projectSchema.ts:23-33`：Zod仍不是 Manager + pinned Provisioner 的完整 admission mirror。** 可复现的 renderer漏项是 `httpsMode:'caddy'` 没有强制 `services.gateway=true`：`services` refine只看 Auth/REST/Studio/Realtime/Storage/Functions，aggregate refine也没有 Caddy规则，而 `apps/provisioner/internal/render/services.go:10-12`会拒绝该配置。上面的 Direct DB zero-port也是同类漏项。secret truth table也仍与 Go不同：enabled SMTP/Phone/OAuth在 `secretSet:true, secret:{action:''}` 时被 Zod接受，Manager `validateAuth`/`validatePhone`要求已有 secret必须使用 `retain` 或 `replace`；create又不能使用 `retain`，说明 create schema应拒绝客户端伪造的 `secretSet`，而不是把 server-derived marker当成授权。另有 raw IPv6 hostname等双向差异。当前新增 tests只验证若干选定例子，并未证明报告所称的完整 parity。

- **Important — `apps/web/src/features/projects/AuthStep.tsx:20-23`：RHF会阻止非法 submit，但若干真实 nested error仍没有任何可见的 field message。** 例如先关闭 “Allow signup”（同步 `disableSignup=true`），再打开 Anonymous sign-in，Zod把错误放在 `auth.disableSignup`，而该字段没有控件/`FieldError`，所有 `Toggle`也不接收 error，用户只会被卡住。Redirect URL错误位于 `redirectUrls[index]`，第 21 行却只读取数组根的 `.message`；SMTP/Phone/OAuth password输入空白时，secret schema把错误放在 `secret.value`，`SecretField`只读取 `secret.message`。`stepForError`能把用户送回 Auth step并不等于对应字段展示了原因。上一轮 nested-error Important只部分关闭。

- **Important — `apps/web/src/api/types.ts:25-47`：create/update/redacted虽然改了名字，wire DTO仍非 exact Go JSON model。** Go redaction总会把 secret结构清为 `{action:""}`，但 `RedactedSecretInput.action`仍允许 `retain/replace/remove`；更直接的是 Go `omitempty` 会从 redacted GET JSON省略空的 Phone `provider/fields`、redirect URLs、OAuth map、Functions variables、database extensions、network certificate/internal port等字段，而这些在 `RedactedProjectConfiguration`下仍大多必填。`WithSecret`只替换 secret leaf，无法修正这些 projection差异，也没有用 discriminated union静态表达 `replace`才有 value。Task 8若消费当前 alias，仍会把服务端合法的缺省 redacted response当成一个静态上“不可能”的对象。

- **Important — `apps/web/src/features/operations/OperationPanel.test.tsx:17-35`：新增 success/terminal tests没有证明标题声称的 exactly-once、awaited-order和 failure no-navigation。** success test只检查最终 pathname并验证 invalidation“曾被调用”，没有导航 spy/history计数，重复的 pathname断言不能发现 replace被调用两次；`invalidateQueries`立即 resolve，也不能证明 navigation等待其完成。三个 terminal用例的 `waitFor('/projects')`在首次 render、operation fetch尚未 resolve时就已经成立，测试会立即结束并 unmount，即使 terminal response随后错误导航也会通过。代码审查显示当前 effect确实按 operation ID去重、await invalidation且只在 SUCCEEDED导航，但 Task 7 Step 1要求的是能锁住该行为的回归测试；这些测试仍会对关键回归给出假阳性。Wizard tests也仍未从真实控件证明 Standard/Custom、SMTP/OAuth/Phone/Storage及完整 normalized POST，Manager create contract仍被 mock 202隔离。

## Round 2 findings resolved

- 上一轮 Critical 已关闭：`defaultConfiguration`恢复 Go-compatible的空 secret action，`localPath`恢复空默认；POST boundary移除 non-replace plaintext并把 update-only `retain`正规化为空。默认 Lightweight/Standard/Full/Custom aggregate可通过当前前端 schema及 Manager create validation，initial secret encryption不会再因默认 marker报错。
- `Form`现在是 RHF `FormProvider` composition并提供 `FormField/FormControl/FormMessage`，`Alert`也恢复 generated variants/Title/Description contract；六步实际使用 Card、Field/Form、Switch、Select、Collapsible、Input、Textarea和Button。Round 1伪 primitive finding关闭。
- preset选择现在重置完整 default aggregate并仅保留 general identity；Auth/service drift被消除，Storage/Functions/Realtime/DB/pooler/network默认一起回到对应 Go preset defaults。Direct DB/host port runtime问题见 Important。
- Phone已渲染 Twilio `verifySid`，且 disabled provider不再要求 enabled-only字段；Storage backend正规化、independent S3 API、OAuth entry初始化与 Functions array errors均已改善。
- renderer不支持的 extensions和非 0/8000 internal gateway已在 schema拒绝；Manual HTTPS仍未暴露。Caddy、Direct DB和secret状态差异见 Important。
- Review已将零 Functions变量正确显示为 `0 variables`，完整类别汇总不渲染 secret plaintext。Round 1 Review Minor关闭。
- 报告现在正确记录实际 fix commit `0841220`；Round 1 hash Minor关闭。旧 fixed Lightweight POST路径仍不存在，create继续提交 authoritative complete aggregate及兼容的顶层 projection。
- OperationPanel实现本身保留 operation/create project ID，await `['projects']` invalidation后单一 owner replace-navigate，并以 active operation ID去重；FAILED/ROLLED_BACK/CANCELLED不进入 success effect。测试证据缺口见 Important。

## Round 2 verification

- 从 `9f1bea0` 独立导出快照运行 `vitest --run`：PASS，10 files / 34 tests。
- 同一独立快照运行 TypeScript check与 Vite build：PASS；Vite仅报告约 775 kB chunk warning。
- `git diff --check 5bcb974..9f1bea0`：PASS。
- 当前共享工作树已经前进到更晚提交且有其他代理改动；以上代码行、判断和测试均以 `git show 9f1bea0`/独立快照为准，未将后续修复计入本轮。

---

# Task 7 Fix Round 2 Addendum — CHANGES REQUIRED

增量范围：`9f1bea0..b09d779`（实现提交 `16cd2da`、`02f65a2`，另含两次 docs-only提交）。本 addendum只审计这 4 个文件的真实增量，不接受新增报告文字作为证据。结论：Round 2 的 5 个 Important均未完全关闭，并新增 1 个 Important交互回归；因此结论仍是 `CHANGES REQUIRED`。

## Important

- **Important — `apps/web/src/features/projects/PresetStep.tsx:38`：新增的 Gateway “required”一行不是 dependency closure，并能把用户锁进一个 disabled + false 的非法状态。** 新逻辑只在某依赖服务已经开启时禁用 Gateway switch；`setServiceEnabled()`仍不会在 Auth/REST/Studio/Realtime/Storage/Functions从 off 切回 on 时同步开启 Gateway。可复现路径是先关闭所有 public服务，再关闭 Gateway，然后重新打开 Functions（或 Auth等）：下一次 render的 `gatewayRequired`为 true，于是仍为 false的 Gateway控件立刻变成 disabled，schema报错但用户不能直接修复，只能再次关闭依赖、手动开 Gateway、再重开依赖。真正 closure应在每个依赖服务 mutation中原子恢复 Gateway，或让 disabled值本身始终满足约束。该行也完全没有覆盖 `httpsMode:'caddy'` 的 renderer依赖，因为它只读取 `services`。

- **Important — `apps/web/src/api/types.ts:43-57`：DTO拆分有所进展，但新的 redacted secret optionality错误理解了 Go `omitempty`，仍不是 exact wire model。** Go中的 SMTP/Phone/OAuth/Storage/Function secret都是非指针 `SecretInput` struct；标准 `encoding/json` 的 `omitempty`不会省略零值 struct，所以 Manager redaction后实际编码的是 `secret/password/value: {"action":""}`，不是字段缺失。新增 `password?`、`secret?`、`value?`因此制造了新的静态假象。同时 `RedactedSecretInput.action`仍允许 `retain/replace/remove`，而 `redactConfiguration()`总是清成空 action；create/update secret仍用“action union + optional value”而非 `replace`必须带 value的 discriminated union。Phone provider/fields、redirect URLs、OAuth map、variables、extensions等真正 collection/string `omitempty`现已正确建模为 optional，且 editable aggregate终于与 redacted projection分离，但 Round 2 DTO Important仍未关闭。

## Round 2 open-status after addendum

- **Direct DB / Manager-owned ports：未处理。** 本增量未改 `setServiceEnabled('directDb')`、schema、Manager allocator或Provisioner；Direct DB仍提交 service=true + zero ports并在 render失败，Standard/Full固定 Supavisor host ports仍无 initial create reservation。
- **Zod ↔ Go/renderer parity：未处理。** `projectSchema.ts`没有变化；Caddy-without-Gateway、DirectDB-zero-port和 create `secretSet:true/action:""`差异仍存在。新增 Gateway UI禁用不构成 API admission规则，且存在上述新回归。
- **Nested RHF errors：未处理。** AuthStep没有变化；`disableSignup`、redirect array item和 `secret.value`错误仍没有对应可见 message。
- **Exact DTO：部分处理但仍 open。** 真正的 `omitempty` collections以及 editable/redacted结构已改善；non-pointer secret optionality和action/value truth table仍不准确，详见 Important。
- **Operation/wizard regression tests：未处理。** 本增量没有 test变更；terminal no-navigation仍在 fetch完成前通过，success仍未证明 exactly-once/await顺序，service closure和DTO也没有新增行为/type contract测试。

## Other incremental checks

- `StorageFunctionsStep.tsx:17`现在在 R2时清空 region、只为 generic S3保留 endpoint，并为非 R2清空 account ID；这使 backend切换更确定，未发现该两行引入新的 create-invalid对象字段残留。
- 从 `b09d779` 独立导出快照运行 `vitest --run`：PASS，10 files / 34 tests；TypeScript check与 Vite build：PASS（仅约 775 kB chunk warning）。这些仍是相同 34 个 tests，不能覆盖上述未测状态转换。
- `git diff --check 9f1bea0..b09d779`：PASS。未修改实现代码，未提交本报告。

---

# Task 7 Fix Round 3 Review — CHANGES REQUIRED

复审范围：`b09d779..50d9675`（实现提交 `add061a`、`60ae3c4`，另含 docs-only提交）。本轮按用户的 authoritative-only硬规则扫描 Go contract、HTTP decode、Store create、installer allocation及前端 create DTO，并逐项回归 Round 2 addendum的 6 个 Important。全端口分配、create secret action和 Operation success测试已有实质修复，但仍发现 6 个 Important；因此本轮不予 `APPROVED`。

## Important

- **Important — `internal/contracts/project.go:54-60`、`apps/manager/internal/project/validate.go:38-43`、`apps/web/src/api/types.ts:59` 与 `NewProjectPage.tsx:28`：create contract仍保留第二份顶层 `supabaseVersion`，没有做到“只以 configuration 为 authoritative”。** Domain、Site URL和Services的旧 projection确已删除，Store也从 aggregate派生 SQL projection；但 `ProjectDraft`/`CreateProjectRequest`仍要求 `supabaseVersion`，前端仍从 `configuration.general.supabaseVersion`复制一份，Manager还要求两份相等。只提交完整 authoritative configuration的客户端会因顶层零值而收到 422。版本与之前三个 projection一样已存在于 `configuration.general`，应删除顶层字段和 equality branch，而不是保留一个必须同步的双轨值。

- **Important — `apps/manager/internal/install/orchestrator.go:58-62`：installer仍有被硬规则明确禁止的 silent aggregate fallback，并可能用稀疏 projection覆盖 desired configuration。** `GetDesiredConfiguration`只在 `readErr == nil`时采用 aggregate；任何 missing/corrupt/transient read error都会被静默忽略，随后用 `Project`的 General/Services projection构造一个其余 section全为零值的配置，分配端口并调用 `PersistAllocatedConfiguration`。如果 row存在但 JSON decode失败或一次读取异常恢复，后一步可把 Auth、Storage、Realtime、Functions、Database与Pooler desired state整体覆盖为稀疏 fallback。Authoritative-only路径必须在 aggregate读取失败时让 operation失败/回滚，不能继续旧 projection fallback。

- **Important — `apps/web/src/features/projects/DatabaseNetworkStep.tsx:18` 与 `apps/manager/internal/install/orchestrator.go:166-170`：Supavisor transaction/session仍是可编辑配置，但 Manager无条件丢弃用户提交值。** 页面用普通 NumberField让用户编辑并在 Review/POST中呈现 `pooler.transactionPort`/`sessionPort`（defaults仍是 6543/6544），新的 installer随后总是用 allocator结果覆盖两者。底层现在确实会为每个项目分配唯一 transaction/session host port，且 renderer会使用回写值，这关闭了碰撞/必败主问题；但当前 typed form承诺的两个用户设置是假的。若这些端口是 server-owned，create UI/DTO应提交 0并只读显示“Allocated by Manager”；若允许 requested port，则 allocator必须验证并尊重非零请求，不能静默改写。

- **Important — `apps/web/src/features/projects/PresetStep.tsx:15-27`：dependency action仍缺 Storage → REST closure，报告所称“full dependency closure”不成立。** 开启 Storage会恢复 Database和Gateway，却不恢复 REST；开启 imgproxy只额外打开Storage，同样不恢复 REST。可复现路径：先关闭Storage与REST，再重新开启Storage（或 imgproxy），得到 `storage=true/rest=false`；schema/Manager会拒绝，用户仍需发现并手动打开REST。Functions/Caddy/Gateway trap、Studio/meta、logs/vector和DirectDB/database已修复，但 Storage的强依赖必须在同一 mutation中原子闭合。

- **Important — `apps/web/src/features/projects/OAuthProviderFields.tsx:12-14` 与 `StorageFunctionsStep.tsx:16-22`：nested secret errors仍只在 Auth helper修复，OAuth和Storage会继续无反馈阻塞。** create secret schema现在把 whitespace replacement错误附在 `.secret.value`/`.secretAccessKey.value`；AuthStep的 `fieldError`新增 `value?.message`后能显示 SMTP/Phone错误，但 OAuth `getError`与Storage `error`仍只返回根对象的 `.message`。输入仅空格的 OAuth client secret或object-storage secret会使 RHF拒绝 Review/submit，同时相应 password field没有错误文案。Redirect item和`disableSignup`错误已修复，但 Round 2 nested-error Important尚未完全关闭。

- **Important — `apps/web/src/api/types.ts:24-58` 与 `apps/manager/internal/project/configuration_service.go:132-163`：redacted/create secret DTO已精确化，但 update DTO仍不匹配真实 Go PATCH contract。** Redacted non-pointer secrets现在正确要求 `{action:""}`，create只允许 empty/replace且 replace要求value，Manager也在插入project row前拒绝retain/remove，这些修复是正确的。可是 backend明确支持把 unchanged redacted snapshot的 `action:""`作为 update输入，并在对应 `*Set`为true时正规化成retain；`UpdateSecretInput`却只允许retain/remove/replace，排除了合法的空 action。要么 update wire DTO包含 empty marker，要么 API删除该兼容语义并要求显式retain；当前仍不能称 exact update contract。

## Minor

- **Minor — `apps/web/src/features/projects/projectSchema.ts:13-27`：domain parity仍非 exact。** Frontend把任何四段数字样式先解释为IPv4并拒绝超255/前导零，例如 `999.999.999.999`；Go `validDomain`在 `net.ParseIP`失败后会把它按四个合法DNS label接受。Frontend也保留了Go没有的 JWT expiry上限和更严格的 email parser。收紧输入本身未产生服务端失败，但“IPv4/IPv6/DNS validation matches Go”的报告表述不准确；应共享规则或补齐Manager约束。

## Round 3 findings resolved

- **全端口分配核心问题已关闭。** `ReserveMany`以SQLite transaction原子claim API、Studio、Direct DB、Supavisor transaction/session候选；Direct DB同步 `services.directDb`、`database.directPort/directPortNumber`和`network.directDatabasePort`，Supavisor分配值进入 `pooler`，随后 aggregate及canonical projection在一个Store transaction中回写，并以同一内存配置交给Provisioner。代码路径满足renderer的Direct DB与Supavisor端口约束；失败的multi-port claim不会留下部分新set。UI静默丢弃requested pooler值见 Important。
- **API-only install allocator旧路径已删除。** 原先 `Reserve(KindAPI)`的单端口接口/调用已移除；configuration orchestrator中对既有 API reservation的读取用于后续reconcile hydration，不是另一个allocator create轨。`CreateProject`也不再接受可选configuration varargs，`NormalizeDraft`、`configurationSupplied`、`firstConfiguration`均已删除。剩余 installer fallback见 Important。
- **大部分 service/Zod admission已对齐。** Manager新增Functions/Caddy/Storage/Realtime/Supavisor/DirectDB/Logs依赖与Auth/service equality；前端新增Caddy和DirectDB boolean relation。Caddy选择会开启Gateway，重新开启public service也会恢复Database/Gateway。Storage→REST漏项见 Important；少数双向domain/bounds差异见 Minor。
- **create secret安全语义已关闭。** TS/Zod create只允许empty/replace，replace静态和运行时都要求value；normalizer移除non-replace plaintext；Manager initial create只接受empty/replace、忽略伪造set marker、拒绝retain/remove且在失败时不留下project row。未发现create secret plaintext进入redacted snapshot/report。
- **Operation regression tests finding已关闭。** success test用deferred invalidation证明callback发生在invalidate完成之后，并分别断言invalidate/callback exactly once及operation project ID；terminal cases先等待真实FAILED/ROLLED_BACK/CANCELLED response渲染再断言未离开 `/projects`。实现继续只在SUCCEEDED进入success effect，NewProject callback使用replace navigation。
- **RHF nested errors部分关闭。** `disableSignup`、secure-email relation、redirect array item和Auth SMTP/Phone `.value`错误现已显示；OAuth/Storage残留见 Important。
- **DTO finding部分关闭。** 真正的Go `omitempty` collections已optional，non-pointer redacted secret已恢复required empty action，create discriminated union已精确；top-level version双轨与update empty action残留分别见 Important。

## Round 3 verification

- 从 `50d9675` 独立导出快照运行 `go test ./...`：PASS（Manager、Provisioner及integration suites全部通过）。
- 同一快照运行 `vitest --run`：PASS，10 files / 38 tests。
- 同一快照运行 TypeScript check与 Vite build：PASS；Vite仅报告约 778 kB chunk warning。
- `git diff --check b09d779..50d9675`：PASS。
- 审计未修改实现代码，未提交本报告。

---

# Task 7 Fix Round 4 Review — CHANGES REQUIRED

复审范围：`50d9675..f2cb32f`（实现提交 `1228b35`、`8a8f595`，另含 docs-only提交）。本轮逐项回归 Round 3 的 6 个 Important 与 1 个 Minor，并在 `f2cb32f` 独立快照验证 Go、Web和build。authoritative create、installer读取、server-owned pooler、Storage closure及nested errors均有实质修复，但 update secret路径仍有 2 个 Important，且 domain parity Minor未关闭；因此本轮不予 `APPROVED`。

## Important

- **Important — `apps/manager/internal/project/configuration_service.go:66-108`：删除隐式 retain 后，任何未携带 secret section 的 partial patch都可能被未修改的已配置 secret阻断。** `PreparePatch`先从 redacted desired aggregate取完整 base；持久化快照中的已配置 secret是 `*Set=true, action:""`。新 `requireExplicitSecretActionsForPatch`只检查请求实际携带的 section，这一点正确；但紧接着 `ValidateConfiguration(cfg)`仍验证合并后的完整 aggregate，并要求 enabled SMTP/Phone/OAuth的已有 secret action必须是 `retain`或`replace`。因此项目只要有一个 enabled configured OAuth/SMTP/Phone，像 `{expectedRevision:1,general:{...}}` 这样的合法 General-only patch也会返回 validation error，尽管客户端既没有读取明文也没有修改该 secret。独立审计测试先通过真实 create持久化 Google secret，再调用 General-only `Patch`，稳定失败为 `auth.oauth.google.secret: an existing secret must use retain or replace`。显式动作应只约束请求覆盖的 secret leaf；被省略section必须保留现有 secret并能通过合并后验证，不能恢复接受客户端空 action作为显式 retain的旧双语义。

- **Important — `apps/web/src/api/types.ts:23-31,63`：新的 `UpdateProjectConfiguration`仍不是 Go PATCH wire contract，且 helper会为默认 Local Storage生成服务端必拒值。** `WithSecret<ProjectConfiguration, UpdateSecretInput>`把完整 update aggregate中的每个 secret leaf都强制为 `retain/remove/replace`；然而 Go只在对应 `*Set=true`时要求显式动作，未配置 secret必须能够保持 `action:""`。这不只是 optionality差异：默认 Local Storage要求 `secretAccessKeySet=false`且 `secretAccessKey.action==""`，任何 `retain/remove/replace`都会被 `validateStorage`以“local storage cannot include object-storage credentials”拒绝。新增 `toUpdateSecretInput(..., false)`却返回 `{action:'remove'}`，正好生成这个非法值；同样，disabled且无provider的 Phone若被转成 remove，会绕过空配置early-return并触发provider错误。因此当前 TS类型无法表示一个后端接受的默认 full-configuration update，也无法精确表达“configured secret必须显式动作、unset secret保持空 marker”的单轨规则。新增测试只断言 helper自身返回remove，没有将结果送进Go validation，未捕获合同断裂。

## Minor

- **Minor — `apps/web/src/features/projects/projectSchema.ts:23-25` 与 `apps/manager/internal/project/validate.go:50-70`：domain admission仍未与Go对齐，新增测试反而固化了差异。** 前端将四段数字形式优先视为IPv4并拒绝 `999.999.999.999`；Go在 `net.ParseIP`失败后按四个合法DNS label接受它。`projectSchema.test.ts:44-50`现在明确断言前端拒绝，却没有相应收紧Manager，因此不能证明 parity。JWT expiry上下界本轮已补入Go；前端 `z.email()` 与Go `mail.ParseAddress`仍有收紧差异（例如display-name address），若产品选择更严格前端规则也应在Manager使用同一 admission规则。

## Round 4 findings resolved

- **authoritative create已关闭。** `ProjectDraft`与`CreateProjectRequest`删除顶层 `supabaseVersion`，New Project只发送 `configuration.general.supabaseVersion`；HTTP strict decoder会把旧顶层字段当unknown JSON拒绝。Project response/SQL projection保留 `supabaseVersion`只是aggregate派生的read projection，不是第二条create authority。
- **installer fallback已关闭。** `Run`读取 `GetDesiredConfiguration`失败会进入 `LOAD_CONFIGURATION` rollback，不再用Project projection构造稀疏aggregate；新增orchestrator test删除aggregate row并证明不会调用reconcile fallback。
- **pooler server ownership已关闭。** Go与Web preset defaults均把 transaction/session设为0；Database/Network step用read-only controls显示“Allocated by Manager”，create schema允许两者同时为0，安装时仍由atomic multi-port allocator分配、回写并交给renderer。没有发现本轮破坏已有all-selected allocation transaction。
- **Storage closure已关闭。** `setServiceEnabled`开启Storage或imgproxy时同步恢复REST；Storage开启时REST switch被标为required且不能关闭，新增真实wizard test覆盖Storage关闭REST后重新开启的DB/REST/Gateway closure。
- **nested secret errors已关闭实现问题。** OAuth与Storage error helper现在读取根 `.message` 或 `.value.message`，whitespace replacement能在对应secret control显示；新增UI test覆盖OAuth，schema path test同时覆盖SMTP/OAuth/Storage。
- **update空action兼容层已删除，但单轨尚未完整。** backend不再把客户端提交的configured `action:""`正规化为retain，新增显式动作检查；TS redacted/create/update leaf仍分离。partial patch与unset update类型问题见Important。
- **Operation success行为保持通过。** 本轮未改OperationPanel；现有deferred invalidation test继续证明SUCCEEDED在invalidation完成后只调用一次callback并使用operation project ID，FAILED/ROLLED_BACK/CANCELLED tests等待真实terminal response后确认不导航。

## Round 4 verification

- 从 `f2cb32f` 独立导出快照运行 `go test ./...`：PASS（Manager、Provisioner及integration suites全部通过）。
- 同一快照运行 `vitest --run`：PASS，10 files / 43 tests。
- 同一快照运行 TypeScript check与 Vite build：PASS；Vite仅报告约 778 kB chunk warning。
- `git diff --check 50d9675..f2cb32f`：PASS。
- 独立临时Go回归探针（未写入工作树）证明 General-only patch在存在未修改enabled Google secret时失败；现有suite没有该partial-section场景。
- 审计未修改实现代码，未提交本报告。

---

# Task 7 Fix Round 5/5 Final Review — NOT APPROVED (LOAD-BEARING)

最终复审范围：`f2cb32f..8a5b6a4`（实现提交 `8bfe0a3`、测试提交 `8a5b6a4`，另含 docs-only提交）。Round 4 的 General-only partial patch与unset Local/Phone update DTO主体已修复，domain numeric-label也已对齐；但真实公开 SMTP/OAuth section API仍有一个 load-bearing secret-update断裂，不能 `APPROVED`。另有两个不阻断当前create主路径、可明确park的契约/parity Minor。

## Load-bearing

- **Important / load-bearing — `apps/manager/internal/httpapi/configuration.go:87-95,99-123` 与 `apps/manager/internal/project/configuration_service.go:141-219`：SMTP或单个OAuth更新会把服务端合并的其他Auth secret误判为客户端隐式retain，项目配置两个Auth secret后细粒度API稳定不可用。** `/configuration/smtp`并不只把SMTP leaf交给service：handler先读取redacted aggregate、替换SMTP，再把整个 `Auth`作为incoming patch；`/configuration/oauth/{provider}`同样把目标provider写入redacted Auth后提交整个section。只要同一项目还配置了另一个OAuth/SMTP/Phone secret，它在该服务端合并快照中就是 `secretSet=true, action:""`。`requireExplicitAuthSecretActions`无法区分目标leaf与handler合并的untouched sibling，因而在 `restoreUntouchedSecretActions`运行前直接返回422。独立审计探针以已配置SMTP+Google项目模拟真实SMTP handler：SMTP自身发送显式retain，Google保持redacted empty marker，`Patch(Auth: &incoming)`稳定失败为 `auth.oauth.google.secret requires explicit retain, remove, or replace action`。反向的单provider OAuth更新在已有SMTP时同理。Round 5 tests只覆盖了整个Auth section都省略的General-only patch，没有覆盖这两个公开subsection handler。修复必须保留单轨边界：只为handler/server合并的untouched sibling内部生成retain，同时仍要求用户实际更新的SMTP/provider leaf显式动作；不能重新接受客户端configured empty action。

## Parkable

- **Minor / parkable — `apps/web/src/features/projects/projectSchema.ts:29` 与 `apps/web/src/api/types.ts:24-33`：update Zod leaf仍拒绝合法unset marker，和本轮已修的TS/Go truth table不一致。** `UpdateSecretInput`与helper现在正确让unconfigured unchanged secret保持 `{action:""}`，因此默认Local Storage和空Phone有合法typed值；但导出的 `updateSecretSchema`仍只有retain/remove/replace，`safeParse({action:""})`会失败。当前schema只被tests引用、尚未接入页面或API，所以不阻断Task 7 create flow；在Task 8消费它之前应加入unset branch并用同一truth-table test锁定。

- **Minor / parkable — `apps/manager/internal/project/configuration.go:193-195` 与 `apps/web/src/features/projects/projectSchema.ts:32`：SMTP email只对齐了display-name案例，仍非exact parser parity。** Manager现在正确拒绝 `Bee <bee@example.com>`，但 `mail.ParseAddress`仍接受 `bee@localhost`（独立Go探针PASS），而前端 `z.email()`拒绝它；quoted local part、address literal或一字符TLD也可能继续形成类似差异。前端更严格不会制造renderer失败，故可park，但报告不能声称完整email parity，除非Manager与前端共享同一规则。

## Round 5 findings resolved

- **Round 4 partial General finding主体已关闭。** `restoreUntouchedSecretActions`只在整个Auth/Storage/Functions section未输入时为redacted configured leaves内部恢复retain，随后secret lookup与保存再次清空action；incoming full/section configured empty marker仍由显式动作检查拒绝。新增Go测试覆盖configured SMTP+Google General-only patch及redacted response。
- **unset/helper/Local Storage/Phone finding已关闭。** TS update union加入 `{action:""}`；helper对unconfigured unchanged返回empty、configured unchanged返回retain、显式删除configured返回remove。Go测试证明默认Local Storage与disabled empty Phone不会被General-only patch错误转成remove或触发validation。
- **domain numeric-label parity已关闭。** 前端不再在IPv4 parse失败后提前拒绝四段数字DNS label，`999.999.999.999`与Go均接受；IPv6/localhost测试保持通过。
- **email parity部分修复。** Manager新增JWT-range既有约束与display-name拒绝测试，前后端对该案例一致；剩余parser差异见parkable Minor。
- 本轮没有改create DTO、installer、port allocator、service closure、nested error、Review或OperationPanel；此前已关闭的对应行为未见增量回归。

## Final verification

- 从 `8a5b6a4` clean独立快照运行 `go test ./...`：PASS（全部Manager、Provisioner及integration packages）。
- 同一提交独立运行 `vitest --run`：PASS，10 files / 44 tests。
- 同一提交运行TypeScript check与Vite build：PASS；仅既有约778 kB chunk warning。
- `git diff --check f2cb32f..8a5b6a4`：PASS。
- 独立临时Go探针（未写入工作树）复现SMTP-style Auth patch被untouched Google secret拒绝；另一个探针证明Manager接受前端拒绝的 `bee@localhost`。
- 最终cap分类：1个load-bearing Important阻止approval；2个parkable Minor不阻断当前create主路径。未修改实现代码，未提交本报告。
