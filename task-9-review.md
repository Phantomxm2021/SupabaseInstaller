# Task 9 严格只读审计

审计范围：`005dd0c..eae4da9`

裁决：**CHANGES REQUIRED / NOT APPROVED**

没有发现 Critical，但存在 5 个 Important 和 1 个 Minor。Task 9 不能宣称“complete shadcn interface migration”：目标旧 `.panel/.button/.badge`、raw dialog、raw progress 的直接消费者确实已清零，但仍有同类旧 CSS/旧表单/旧导航轨道，并且移除旧 `.alert` CSS 时漏掉了最后一个生产消费者。Task 8 的四个携带阻断中，三个实现路径有实质修复；Gateway 的直接案例已修，但同一依赖闭包仍有生产级不对称残留，且 OAuth field-error、disabled-owner preview、`disableSignup` 等关键路径没有有效的端到端组件行为测试。

## Critical

无。

## Important

### I1. Services 依赖闭包仍是不对称的：启用 Studio 不会恢复 Gateway，关闭 Studio 又会误关可独立存在的 postgres-meta

位置：

- `apps/web/src/features/project/configuration/ServicesSection.tsx:16`
- `apps/web/src/features/project/configuration/ServicesSection.tsx:23`
- `apps/web/src/features/project/configuration/ServicesSection.tsx:29`
- 对照权威校验：`apps/web/src/features/projects/projectSchema.ts:31`、`apps/manager/internal/project/configuration.go:124-128`

复现：

1. 以合法的 `gateway=false, studio=false, postgresMeta=true` 配置进入 Services。
2. 打开 Studio。第 16 行只设置 `postgresMeta=true`，没有设置 `gateway=true`；随后 Zod/Manager 会以“API Gateway is required by enabled services”阻止保存，而不是按 PRD 的 UI dependency engine 自动补齐依赖。
3. 反向以 `studio=true, postgresMeta=true` 进入，关闭 Studio。第 23 行无条件把 `postgresMeta=false`，即使 postgres-meta 是独立可配置服务且“Studio requires postgres-meta”只规定正向依赖。

影响：Task 8 携带的 Gateway/postgres-meta 直接案例只修了第 29 行的 Gateway 批量关闭，但同类“关闭 dependent 时误关 dependency / 打开 dependent 时不补 dependency”仍存在。管理员会丢失独立 postgres-meta 意图，或被 UI 制造出的无效组合阻断。`ConfigurationPage.test.tsx:122-138` 只验证“关闭 Gateway”后的瞬时 switch 状态，因此对这两个方向都是假阴性盲区。

### I2. shadcn 迁移仍有多条旧轨；旧 Alert CSS 甚至在最后消费者迁移前就被删除

位置：

- `apps/web/src/features/settings/ManagerSettingsPage.tsx:21`
- `apps/web/src/features/project/DeleteProjectDialog.tsx:47-55`
- `apps/web/src/features/project/ProjectLayout.tsx:15`
- `apps/web/src/features/projects/ProjectsPage.tsx:12`
- `apps/web/src/styles.css:70-80, 98, 113-132`

复现与证据：

- `styles.css` 在本提交删除了 `.alert`/`.alert.error`，但 Settings 的 query error 仍渲染 `<div className="alert error">`。因此 session 请求失败时只剩未样式化、无 `role=alert` 的普通 div。这正违反计划“Remove CSS rules only after their last consumer is migrated”。现有 Settings 测试只覆盖 200 响应。
- 删除确认虽然外壳是 `AlertDialog`，内部两个 radio 和确认文本框仍是 raw `<input>`，并由 `.delete-options` 旧 CSS 驱动；确认输入没有仓库 `Input` 的 focus ring/token/invalid 语义。全生产 feature 搜索仍得到 3 个 raw form controls，全部在这里。
- 项目内导航仍是 ad-hoc `<aside>` + 13 个 `NavLink`，由 `.project-shell/.project-nav` 旧 CSS 驱动，而批准设计明确要求 shadcn `Sidebar` 覆盖 global **and project navigation**。在 <=900px 时仅用 `font-size:0` 收缩文本，没有 shadcn Sidebar 的 tooltip、keyboard/focus composition 或 ScrollArea。
- Projects query error 只是把旧 `.alert.error` 换成 `<div className="p-4 text-sm text-destructive">`，没有使用仓库 `Alert`，也没有 live/alert role；这是换类名而非语义迁移。
- `styles.css:70-76` 的 `.sidebar`、`:98` 的 `.wizard-error`、`:122` 的 `.metric-value` 已无生产消费者，仍是明确的旧/死 CSS 残留。

影响：界面不是一条权威 shadcn 轨道，错误态和键盘焦点体验因页面而异；Settings 失败态已经发生可见回归。用户的硬要求是彻底删除旧逻辑而不是包壳/换类名，因此此项阻断批准。

### I3. 配置页 12 个 tabs 在 768px 和更窄宽度被固定高度压叠

位置：

- `apps/web/src/features/project/configuration/ConfigurationPage.tsx:49`
- 基础 primitive：`apps/web/src/components/ui/tabs.tsx:26-28, 56-64`

复现：

1. 在 768px 打开任意 `/projects/:id/configuration`。
2. `md:grid-cols-6` 会把 12 个 trigger 排成两行；更窄时 `grid-cols-3` 排成四行。
3. 该 `TabsList` 没有 wizard 已使用的 `h-auto`，因此仍继承官方 primitive 的 `group-data-horizontal/tabs:h-8`。每个 `TabsTrigger` 又使用 `h-[calc(100%-1px)]`，多行内容会压叠/溢出 32px 容器。

影响：计划 Step 4 明确要求在 768px 验证 configuration overflow。管理员无法可靠点击/辨认配置分区，焦点 ring 也会重叠。当前测试只断言 tab 文本存在，没有 viewport/layout 断言。

### I4. Login/Setup 没有完成计划要求的可访问 descriptions 与 field-error 关联

位置：

- `apps/web/src/features/auth/LoginPage.tsx:14`
- `apps/web/src/features/auth/SetupPage.tsx:18`
- `apps/web/src/features/auth/SetupPage.test.tsx:45-51`

复现：

- Login 的 Username 没有 description；Password 虽渲染 `FieldDescription`，但 description 没有 id，Input 也没有 `aria-describedby`，因此 accessibility tree 中并不是输入框描述。
- Setup 只给 Password 连接了 `password-hint`；Username 和 Confirm password 没有 description。
- Setup 的三个 `FieldError` 都没有 id，input 的 `aria-describedby` 也不包含 error；Password 出错时仍只描述 hint。`role=alert` 可能宣布新错误，但聚焦字段时无法取得对应错误说明。
- 新增测试只检查 Setup Password hint，没有 Login 测试、Username/Confirm description 或 error association 测试。

影响：直接不满足 Task 9 Step 1 的“setup/login fields have labels and descriptions”和设计的 labels/descriptions/field errors 验收要求。屏幕阅读器用户无法在重新聚焦字段时理解错误。

### I5. 四个 Task 8 携带阻断的新增测试多为局部/结构断言，不能证明真实提交流程

位置：

- `apps/web/src/features/project/ConfigurationPage.test.tsx:71-104, 122-151`
- `apps/web/src/features/project/configuration/types.test.ts:76-91`
- `apps/web/src/features/project/LifecycleActions.test.tsx:70-77`
- `apps/web/src/features/projects/ProjectsPage.test.tsx:14-19`
- `apps/web/src/features/project/OverviewPage.test.tsx:20-25`

具体缺口：

- object callback / `setError`：只有 General 422 测试；OAuth 测试只断言 endpoint，完全不返回 provider field error，也不验证 `stripFieldPath -> pending.setError -> aria-invalid/error text`。因此 OAuth positional wiring 再次丢失时测试仍可通过。
- Gateway closure：只点击 Gateway 后读 switch 属性，不执行 Save/Confirm，不断言 PATCH body，也不覆盖 I1 的反向闭包。
- disabled-owner preview：三个测试只直接调用 `affectedServices/sectionImpact`，没有渲染配置页、编辑 disabled-owner 字段并断言确认对话框同时显示“Configuration metadata only”和“No runtime restart expected”。UI 传参/组合回归仍可漏过。
- Auth：只覆盖 `phone.provider` 的客户端 Zod 错误；没有 `disableSignup` 可见/可访问断言，也没有服务端嵌套 Auth field error。
- 多个“迁移测试”只检查 `data-slot`，证明组件标签存在但不证明 loading/error/keyboard/dialog/responsive 行为。Lifecycle busy 也只测 STOPPED/start 分支，未覆盖 stop/restart。

影响：65 个测试全绿不能作为四个 load-bearing blocker 已闭合的充分证据；至少需要跨表单提交、确认 dialog、PATCH body/API error、回到字段后的可访问状态行为测试。

## Minor

### M1. 生产 bundle 仍有 834.12 kB 单 chunk 警告

位置：Vite production build output。

影响：不阻断本次语义迁移，但首屏解析成本偏高。可在后续用 route-level dynamic import/manual chunks 处理。

## Task 8 四项携带阻断逐项裁决

| 携带项 | 实现审计 | 测试审计 | 裁决 |
|---|---|---|---|
| typed object callback，`setError` 不丢（含 OAuth） | `SectionSave<T>` 与各 section 的 object input 已落地；OAuth 也把 provider + object 传到 `submit`，mutation error 会调用保存的 callback | 只测 General field error；OAuth 无 422/nested field error 行为测试 | 实现看起来已修，证明不足 |
| Gateway 不强关 postgresMeta 等独立服务 | Gateway 批量关闭已不再包含 postgresMeta/Supavisor/Logs/DirectDB | 只测 Gateway 关闭后的 switch | 直接案例已修，但 I1 的同类闭包残留仍阻断 |
| disabled-owner preview 不出现空 affected + recreate | `sectionImpact`/`affectedServices` 对 General/Network/Pooler owner 做了过滤 | 仅 pure helper 单测，无对话框行为 | helper 已修，集成证明不足 |
| Auth `phone.provider` / `disableSignup` 错误可见可访问 | provider 有 `aria-invalid` + `aria-describedby`；`disableSignup` 被投影到 Allow signup/Phone toggle | 只测 provider；不测 `disableSignup` 或服务端嵌套错误 | provider 已修；其余证明不足 |

## 旧轨残留搜索结果

只读搜索命令与结果摘要：

```text
pattern                         base  head
className="panel                 4     0
className="button                9     0
className="badge                 1     0
dialog-backdrop                  1     0
legacy progress-track            3     0   (排除 generated ui primitive)
raw <dialog>/<progress>           -     0
raw feature form controls         -     3   (全部在 DeleteProjectDialog)
legacy className="alert error"    6     1   (ManagerSettingsPage)
```

结论：指定的五类直接旧消费者有实质删除，不是简单重命名；但搜索同时证明旧表单、旧项目导航、旧 Settings error consumer 和死 CSS 尚未彻底删除，所以整体仍不满足“单轨权威实现”。

## 验证证据

在 `eae4da980fee8d68f01906bd6d938111cbebf093`、干净 worktree 上运行：

```text
npm run test --workspace apps/web -- --run
PASS: 12 files, 65 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2218 modules transformed; build completed
WARN: dist/assets/index-DHIeRe3e.js 834.12 kB (gzip 258.81 kB)

git diff --check 005dd0c..eae4da9
PASS
```

构建后 `git status --short` 仍为空；审计未修改实现、测试或提交。

---

# Round 1 修正复审

新增审计范围：`eae4da9..20deb35`；同时按 `20deb35ba07fa6b6c6289b3c731d2189a24c1ba1` 的完整最终状态重新审计原 I1–I5。

裁决：**CHANGES REQUIRED / NOT APPROVED**

本轮没有发现 Critical。原 I1 的 Studio/Gateway/postgres-meta 直接闭包实现、原 I4 的 Login/Setup aria、Settings/Projects Alert、Delete radio/input 以及三类 Task 8 配置错误/preview 行为测试已有实质修复；但仍存在 3 个生产级 Important，另有 1 个 load-bearing 测试 Important 和 1 个 Minor。尤其是 Services 的新 dirty 写法会把已改回基线的开关继续当作变更，ProjectLayout 在 768px/移动端仍不响应式，并且应用仍明确保留十条 `LegacyConfigurationRedirect` 旧路由兼容轨。

## Critical

无。

## Important

### R1-I1. Services 开关改回原值后 dirty 状态不会清除，会提交无变化 PATCH 并显示“空 affected services + recreate”

位置：

- `apps/web/src/features/project/configuration/ServicesSection.tsx:31`
- `apps/web/src/features/project/configuration/ConfigurationPage.tsx:52,63`
- `apps/web/src/features/project/configuration/types.ts:66-70,81-84`

复现：

1. 进入一个 `directDb=false` 的合法项目，在 Services 打开 “Direct PostgreSQL port”，再立即关闭，使最终表单值回到服务端基线。
2. 第一次变更调用 `setValue(..., { shouldDirty: true })`，React Hook Form 将 `directDb` 写入 `dirtyFields`。第二次回到基线时本实现传入 `shouldDirty: false`；该选项的含义是**不重新计算 dirty**，而不是把 dirty 清为 false，因此旧的 `dirtyFields.directDb=true` 和 `isDirty=true` 被保留。
3. “Save Services” 仍可点击。`dirtyLabels` 认为存在变更，但 `affectedServices` 按 baseline/value 对比得到空数组；`serviceImpact` 对零个实际变化落入 `return 'recreate'`。
4. 确认对话框因此同时显示 “Configuration metadata only” 和 “Runtime recreate required”，确认后还会 PATCH 与原配置相同的整个 Services value，排队一次无意义运行时操作。

影响：这是本轮修正引入/保留的生产行为缺陷，重新制造了 Task 8 明确禁止的“空 affected + recreate”组合，并可能触发无变化 reconcile。正确做法必须让 RHF 对每次值变更都与 default values 重算 dirty（或显式 reset/清 dirty），不能把 `shouldDirty:false` 当作清除指令。

原 I1 的直接闭包本身已修：第 16 行启用 Studio 会同时启用 Gateway/postgres-meta；第 23 行关闭 Studio 不再关闭 postgres-meta；第 29 行关闭 Gateway 只关闭其 public dependents，不会关闭 postgres-meta、Supavisor、Logs/Vector 或 Direct DB。但上述 dirty 缺陷使 Services 的保存语义仍未闭合。

### R1-I2. ProjectLayout 虽使用真实 shadcn Sidebar，但固定 `collapsible="none"` 造成 768px tabs 重叠和移动端内容极窄；旧 `.sidebar` CSS 仍残留

位置：

- `apps/web/src/features/project/ProjectLayout.tsx:13-16`
- `apps/web/src/components/ui/sidebar.tsx:150-180`
- `apps/web/src/hooks/use-mobile.ts:3,10-15`
- `apps/web/src/app/AppShell.tsx:39-40`
- `apps/web/src/features/project/configuration/ConfigurationPage.tsx:60`
- `apps/web/src/components/ui/tabs.tsx:26-28,56-64`
- `apps/web/src/styles.css:115`

复现与布局证据：

- ProjectLayout 现在确实使用仓库生成的 `Sidebar/SidebarMenu`，不是只换标签或仿造 primitive；但传入 `collapsible="none"`。官方 primitive 在该分支于移动判断之前直接返回常驻 div，因此项目 Sidebar 在所有 viewport 永远占 `w-52`（208px），没有 offcanvas、icon collapse 或替代 trigger。
- 在 320px 宽度，global Sidebar 会进入 Sheet，但项目 Sidebar 仍占 208px，Outlet 只剩 112px；`.page` 的 36px 横向 padding 后实际内容仅约 76px。移动端监控和简单操作也无法正常使用。
- 在计划明确要求验证的 768px，`useIsMobile` 判断为 desktop；AppShell 默认展开的 global Sidebar 占 256px，项目 Sidebar 再占 208px，Outlet 仅剩 304px，`.page` 内容约 236px。Configuration 此时由 `md:grid-cols-6` 把一行分成约 36px/格，而 TabsTrigger 强制 `whitespace-nowrap`；“Authentication”“OAuth Providers”“Connection Pooler”等文本必然越界/互相覆盖。新增 `h-auto overflow-visible` 只消除了旧的纵向 32px 裁剪，不能解决实际 768px 横向布局。
- 新测试 `ConfigurationPage.test.tsx:13-18` 只断言 class 字符串存在，jsdom 不计算布局；没有 viewport、scrollWidth/overlap 或 ProjectLayout responsive 行为测试。
- `styles.css:115` 仍保留 `.sidebar ...`、`.sidebar nav ...`、`.sidebar-bottom` 的旧 media 规则；当前 shadcn Sidebar 只有 `data-slot/data-sidebar`，AppShell/ProjectLayout 均没有基础 `sidebar` class，因此这些 selector 已无消费者，是明确死 CSS/旧轨。

影响：原 I2 的项目导航 primitive 迁移只完成了组件来源，没有完成计划要求的响应式组合；原 I3 在 768px 仍是生产级失败。用户要求旧 CSS/消费者彻底删除，残留 media selector 也不能批准。

### R1-I3. 十条 `LegacyConfigurationRedirect` 仍保留旧项目路由兼容轨

位置：

- `apps/web/src/app/router.tsx:47-58`
- `apps/web/src/app/router.tsx:68-71`

复现：访问 `/projects/bee/services`、`/authentication`、`/database`、`/storage`、`/realtime`、`/functions`、`/pooler`、`/network`、`/secrets` 或 `/settings`，应用仍通过命名为 `LegacyConfigurationRedirect` 的组件兼容并重定向到 `/configuration?section=...`。

影响：项目导航在 `ProjectLayout.tsx:6-16` 已全部直接指向唯一 Configuration 路径，这批 routes 不再是当前消费者所需，而是一条完整的旧 URL fallback。虽然早期 Task 8 计划曾允许 redirect，但本轮用户的控制性硬要求明确是“双轨/旧逻辑必须彻底删除，不能兼容或打补丁”；现状直接违反该要求，并且没有测试约束旧轨删除。

### R1-I4. Studio 正反闭包测试仍有假阳性盲区，无法防止 load-bearing 逻辑回退

位置：

- `apps/web/src/features/project/ConfigurationPage.test.tsx:166-181`
- `apps/web/src/features/project/configuration/ServicesSection.tsx:16,23`

证据：名为“enables Studio with Gateway and postgres-meta”的测试在第 171 行把初始 `postgresMeta` 预先设为 `true`。因此即使删除生产代码第 16 行的 `next.postgresMeta = true`，第 176/180 行的 switch/PATCH 断言仍会通过；它只真实证明了 Gateway 从 false 变 true。测试也没有从 `studio=true, postgresMeta=true` 关闭 Studio 并证明 postgresMeta 保持 true，所以第 23 行若恢复原误关逻辑也不会失败。

影响：Gateway disable + PATCH 测试有效，但 Studio 正向 postgres-meta 闭包和反向保留均未被行为测试保护；R1-I1 的“开关改回基线”也完全未覆盖。对于本轮声称关闭的核心携带阻断，这仍属于会产生假绿的 load-bearing 测试缺陷。

## Minor

### R1-M1. 生产 bundle 增长至 841.71 kB 单 chunk

Vite build 通过，但 `dist/assets/index-DD47-j7M.js` 为 841.71 kB（gzip 261.40 kB），继续触发 >500 kB 警告。它不阻断本轮语义修正，但相较首轮 834.12 kB 继续增长。

## 原 I1–I5 逐项复审

| 原项 | Round 1 结果 | 裁决 |
|---|---|---|
| I1 Services closure | Studio enable 已补 Gateway+postgres-meta；Studio disable 保留 postgres-meta；Gateway disable 保留独立服务，且 PATCH body 有断言。但 conditional `shouldDirty` 产生 R1-I1 无变化提交；Studio 测试有 R1-I4 假阳性 | **部分修复，仍阻断** |
| I2 shadcn/旧轨 | Settings/Projects error 已是可访问 Alert；Delete 已用官方 RadioGroup/Input 且旧 delete CSS 删除；ProjectLayout 使用真实 Sidebar。仍有项目 Sidebar 响应式失败、死 `.sidebar` media CSS 和 Legacy route fallback | **部分修复，仍阻断** |
| I3 768px Tabs | `h-auto` 修复纵向多行高度，但 768px 双 Sidebar 后仅约 236px tab 内容宽度，六列 nowrap 文本仍覆盖；测试只是 class 断言 | **仍阻断** |
| I4 Login/Setup aria | Username/Password/Confirm 均有 description；Setup error id 被纳入 `aria-describedby`；Login server error 使用 role=alert 的 Alert，新增测试实际断言关联 | **已修复** |
| I5 关键行为测试 | OAuth 422 nested field、Gateway PATCH、三种 disabled-owner 真实 dialog、phone provider、disableSignup 服务端错误均为真实组件行为测试；Settings/Projects error 也真实走失败响应。Studio 正反闭包与真实 layout 仍有假阳性/缺测 | **大部分修复，仍有 R1-I4** |

## Task 8 四项携带阻断 Round 1 裁决

| 携带项 | 实现与测试证据 | 裁决 |
|---|---|---|
| typed object callback，`setError` 不丢（含 OAuth） | OAuth 测试真实返回 `auth.oauth.google.clientId` 422，经 mutation strip path 和保存的 object callback 回到 Client ID，断言 `aria-invalid`、`aria-describedby`、可见错误 | **已修复** |
| Gateway 不强关独立服务 | 实现保留 postgres-meta/Supavisor/Logs/Direct DB，测试保存确认后断言 PATCH 中 `postgresMeta:true` | **直接项已修复**；Services 仍受 R1-I1/R1-I4 阻断 |
| disabled-owner preview | General/Network/Pooler 测试都真实编辑页面并打开 AlertDialog，同时断言 “Configuration metadata only” 和 “No runtime restart expected” | **原项已修复**；Services toggle-revert 又产生 R1-I1 同类错误组合 |
| Auth provider/disableSignup | phone.provider 客户端错误可见且有关联；真实 422 `auth.disableSignup` 被映射到 Allow signup toggle，并断言错误 id/text | **已修复** |

## Round 1 残留搜索

```text
旧 class consumer：.panel/.button/.badge              0
raw feature <button>/<input>/<select>/<textarea>      0
raw <dialog>/<progress>                               0
Delete .delete-options / project-shell / project-nav  0
legacy className="alert error"                        0
死旧 .sidebar media selector                           1  (styles.css:115)
LegacyConfigurationRedirect routes                    10 (router.tsx:47-58)
```

结论：指定 raw controls 和主要旧 class consumer 的迁移是真实的，不是简单换类名；RadioGroup/Input、Alert、Progress/Dialog 均来自仓库生成 primitives。但旧 CSS 与旧 route fallback 并未彻底删除，且 Sidebar 响应式组合仍未达验收。

## Round 1 验证证据

在 `20deb35ba07fa6b6c6289b3c731d2189a24c1ba1` 上只读运行：

```text
npm run test --workspace apps/web -- --run
PASS: 13 files, 77 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2228 modules transformed; build completed
WARN: dist/assets/index-DD47-j7M.js 841.71 kB (gzip 261.40 kB)

go test ./...
PASS: all Manager, Provisioner, integration packages

git diff --check eae4da9..20deb35
PASS
```

构建后 `git status --short` 仍仅有允许写入的 `?? task-9-review.md`；本轮未修改任何实现、测试或提交。

---

# Round 2 修正复审

新增审计范围：`20deb35..9059455`；最终状态：`90594551644379a2ccc21c7300ce5a21a16a502d`（`fix: complete Task 9 responsive and route cleanup`）。

裁决：**CHANGES REQUIRED / NOT APPROVED**

本轮没有 Critical。R1-I1、R1-I3、R1-I4 已闭合，R1-I2 的移动端、Tabs 与死 CSS 部分已闭合；但桌面 ProjectLayout 把第二个完整 shadcn Sidebar 嵌套在全局 fixed Sidebar 中，打开时会同时覆盖全局栏并再次挤压内容，仍有 1 个生产级 Important。另保留 1 个 bundle Minor。

## Critical

无。

## Important

### R2-I1. 桌面项目 Sidebar 打开后 fixed 面板与全局 Sidebar 重叠，同时本地 gap 再挤压 Outlet；trigger 打开后仍错误标为 “Open”

位置：

- `apps/web/src/app/AppShell.tsx:39-40,70-76`
- `apps/web/src/features/project/ProjectLayout.tsx:13-16`
- `apps/web/src/components/ui/sidebar.tsx:206-248`
- `apps/web/src/features/project/ProjectLayout.test.tsx:6-16`

复现与确定性布局证据：

1. 在任意 desktop/tablet viewport（包括计划要求的 768px）打开 `/projects/:id/overview`。全局 AppShell Sidebar 是第一个 shadcn Sidebar，其 desktop container 由 primitive 渲染为 `fixed ... left-0 ... w-(--sidebar-width)`。
2. 点击 “Open project navigation”。嵌套 ProjectLayout 的第二个 Sidebar 从 `data-collapsible=offcanvas` 切到 expanded。
3. primitive 第 217-225 行的 `sidebar-gap` 从 0 展开为 256px，因此项目 Outlet 被再向右挤 256px；但真正可见的第 227-247 行 `sidebar-container` 是 viewport-relative `fixed left-0`，并不会定位到 AppShell 内容起点。
4. 结果是项目导航面板画在与全局 Sidebar 相同的 viewport `left:0`、相同 `z-10` 位置（后渲染的项目栏覆盖全局栏），而 AppShell 内容区内部留下一个没有对应面板的 256px gap。768px 时全局栏 256px + 本地 gap 256px 后 Outlet 只剩约 256px，再次出现 Round 1 要求消除的双 Sidebar 挤压。
5. 项目 trigger 的 `aria-label="Open project navigation"` 是固定字符串；展开后仍向辅助技术宣布 “Open”，再次点击实际执行的是关闭。测试只断言初始 label 和展开 state，没有验证展开后的可访问名称。

影响：ProjectLayout 确实换成了官方 primitive，默认折叠和 640px Sheet overlay 也有效，但 desktop 组合方式不成立。用户打开导航时会失去全局导航、看到空白挤压区，并在 768px 重新得到极窄配置面板；屏幕阅读器还会收到与动作相反的控制名称。需要让项目导航的 desktop surface 相对 AppShell 内容正确 overlay/定位且不生成第二个 layout gap，或采用适合嵌套导航的单一 Sidebar composition；不能仅以 `defaultOpen=false` 隐藏冲突。

现有 `ProjectLayout.test.tsx:6-16` 在 jsdom 中只检查 `data-state` 和 link 可见，无法计算 fixed 坐标/gap；第 18-30 行只覆盖 640px Sheet。因此名为 “without squeezing” 的证明仅对 mobile 成立，没有覆盖本阻断的 desktop/tablet 路径。

## Minor

### R2-M1. 生产 bundle 仍为 841.38 kB 单 chunk

Vite build 通过，但 `dist/assets/index-C4B-zNbM.js` 为 841.38 kB（gzip 261.37 kB），继续触发 >500 kB 警告。此项不阻断语义修正。

## R1-I1–I4 逐项复审

| Round 1 项 | Round 2 证据 | 裁决 |
|---|---|---|
| R1-I1 RHF 回基线/zero-change | Services 每次 change 后以 `keepDefaultValues:true` reset，使原 server defaults 保持权威并重算 dirty；真实组件测试把 Direct DB 打开再关闭，断言 Save disabled、无 alertdialog、PATCH 计数 0。`serviceImpact` 也对无 dirty/无 changed 明确返回 `none` | **已修复** |
| R1-I2 Sidebar/Tabs/死 CSS | `.sidebar/.sidebar nav/.sidebar-bottom` media 规则已物理删除；Tabs 使用 `flex-nowrap + min-w-max + overflow-x-auto`，不会再把 12 个 nowrap tabs 压进 36px tracks；Base UI Tabs primitive 保留 horizontal arrow-key/focus 语义，聚焦越界 tab 时由原生 scroll container 将其带入视口。移动端项目导航是真实 Sheet，不挤内容。但 desktop 嵌套仍有 R2-I1 | **部分修复，仍阻断** |
| R1-I3 Legacy routes | `LegacyConfigurationRedirect` 函数、useParams import 和十条 routes 已物理删除；旧 URL 匹配明确的 `NotFoundPage`，canonical `/configuration` 匹配 `ConfigurationPage`。生产扫描无其他 compatibility redirect/fallback | **已修复** |
| R1-I4 Studio 测试假阳性 | 正向 baseline 已改为 `postgresMeta=false`，删除 `next.postgresMeta=true` 或 `next.gateway=true` 都会使 switch/PATCH 断言失败；新增反向测试从 Studio+postgres-meta=true 关闭 Studio，断言 UI 和 PATCH 保留 postgres-meta，恢复原误关代码会失败 | **已修复** |

## Tabs、路由与测试有效性补充

- Tabs 的当前 CSS 组合在窄屏可产生真实横向 overflow，而非 `overflow-x-auto` 包在可收缩子项之外；每个 trigger 的 `min-w-max` 阻止文本被压缩。键盘行为没有手写替代逻辑，继续由仓库官方 Base UI Tabs primitive 提供，未发现生产级语义回归。
- `router.test.tsx:7-21` 直接读取注册 route tree 并用 `matchRoutes` 验证 canonical 与十条 removed paths；它不是只搜字符串的假阳性。
- Services pristine 测试真实点击同一 switch 两次并监视 PATCH 次数；Studio 两方向都走实际配置页、确认 dialog 与 PATCH body，能够对指定生产代码回退失败。

## Round 2 全局旧轨扫描

```text
LegacyConfigurationRedirect / production compatibility route    0
十条旧 project child route                                      0
旧 .sidebar/.panel/.button/.badge/.dialog/.progress CSS selector 0
raw feature <button>/<input>/<select>/<textarea>                  0
raw <dialog>/<progress>                                           0
legacy className="alert error"                                   0
delete-options / project-shell / project-nav                      0
```

`sidebar-brand` 和 `sidebar-status` 仍有 AppShell 生产消费者，属于 shadcn Sidebar 内部内容的当前样式，不是无消费者的旧 `.sidebar` 轨道。

## Round 2 验证证据

在 `90594551644379a2ccc21c7300ce5a21a16a502d` 上只读运行：

```text
npm run test --workspace apps/web -- --run
PASS: 15 files, 82 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2228 modules transformed; build completed
WARN: dist/assets/index-C4B-zNbM.js 841.38 kB (gzip 261.37 kB)

go test ./...
PASS: all Manager, Provisioner, integration packages

git diff --check 20deb35..9059455
PASS
```

构建后 `git status --short` 仍仅为允许写入的 `?? task-9-review.md`；Round 2 未修改任何实现、测试或提交。

---

# Round 3 最终只读复审

新增审计范围：`9059455..44cbc59`；最终状态：`44cbc59186b91debd7fec8a167f27fe64c08ae8d`（`fix: unify project navigation in global sidebar`）。

裁决：**CHANGES REQUIRED / NOT APPROVED**

R2-I1 的双 Sidebar/fixed/gap 生产缺陷已真实消除：最终生产代码只有 AppShell 一个 `SidebarProvider`、一个 `Sidebar`、一个 `SidebarInset` 和一个 trigger；ProjectLayout 只渲染 Outlet，desktop/mobile 都复用同一个 global Sidebar。全局旧轨扫描也为零。但项目导航在 bare canonical Configuration route 上仍报告错误的 active state，并且本轮新增的 load-bearing 测试包含恒真断言且没有完整证明要求的 12 links/mobile trigger 状态。因此仍有 2 个 Important；另有 1 个 bundle Minor。

## Critical

无。

## Important

### R3-I1. Bare canonical `/configuration` 展示 General，但项目导航没有任何 section `aria-current`

位置：

- `apps/web/src/features/project/configuration/ConfigurationPage.tsx:38-39,51,60`
- `apps/web/src/app/AppShell.tsx:29-33,74-79`
- `apps/web/src/app/router.test.tsx:17`
- `apps/web/src/app/AppShell.test.tsx:64-75`

复现：

1. 直接访问 router 测试明确认定为 canonical 的 `/projects/bee/configuration`，不带 `section` query。
2. ConfigurationPage 把缺失 query 默认成 `general`，并展示 General 表单；因为 `requested === section === 'general'`，第 51 行不会 canonicalize 到 `?section=general`。
3. AppShell 的 General link active 规则却要求 `new URLSearchParams(location.search).get('section') === 'general'`。当前值为 null，所以 General 没有 `isActive`/`aria-current="page"`；其余 section 也都不 active。

影响：视觉高亮和辅助技术报告的当前位置与实际页面内容不一致，违反本轮“动态项目组准确”和 Task 9 的 a11y 验收。用户从 bookmark、router canonical route 或内部无 query 导航进入时，看到 General 内容但 Sidebar 无当前 section。必须让 General 将缺失 section 视为 active，或统一把 bare route canonicalize 到 `?section=general`；不能让页面与导航各持一套默认规则。

现有测试只在 `?section=services` 断言 Services active，因此无法发现该生产偏差。

### R3-I2. 新 Sidebar 测试含恒真节点断言，且没有证明 12 个 canonical links 与 mobile trigger 的动态名称

位置：

- `apps/web/src/app/AppShell.test.tsx:64-99`
- state 所在 primitive：`apps/web/src/components/ui/sidebar.tsx:206-214,227-239`
- trigger 实现：`apps/web/src/app/AppShell.tsx:119-123`

具体问题：

- 第 74 行查询 `[data-slot="sidebar-container"]` 并断言它没有 `data-state="expanded"`。但 primitive 把 `data-state` 放在外层 `[data-slot="sidebar"]`（第 207-214 行），`sidebar-container` 从不拥有该属性；因此无论 Sidebar expanded/collapsed，此断言都恒真。事实上 AppShell `defaultOpen` 的 desktop Sidebar 初始就是 expanded。
- “12 section canonical links”测试只检查 General、Services、Email & SMTP、OAuth Providers 四个 section 的存在，且完全不检查 href；删除其余八个 section 或把多条 href 指向错误 section，测试仍会通过。
- mobile 测试打开后断言的是 Sheet 内置按钮的通用名称 `Close`，不是 topbar `ResponsiveSidebarTrigger` 的 `Close sidebar`。因此 mobile `openMobile` -> trigger aria-name wiring 回退时测试仍绿。
- 非项目路由测试只覆盖 `/projects`，没有覆盖代码中特判的 `/projects/new` 或 `/settings`。当前生产逻辑对这两条是正确的，但测试不能承担“非项目路由无项目组”的完整回归证明。

影响：单 Sidebar 数量断言（1 Sidebar、1 gap）是真实有效的，也足以证明 R2 的第二 fixed surface 已删除；desktop Close -> Open 测试也有效。但上述断言不能支持本轮声称的完整动态导航/a11y 验收，且一个断言明确检查了错误 DOM owner。用户硬要求“测试不是假阳性”，因此此项阻断最终批准。

## Minor

### R3-M1. 生产 bundle 仍为 841.85 kB 单 chunk

Vite build 通过，但 `dist/assets/index-EvISV62u.js` 为 841.85 kB（gzip 261.63 kB），继续触发 >500 kB 警告。按本轮要求记为 Minor，不作为上述裁决的单独阻断。

## R2-I1 最终复审

已修复，证据如下：

- 生产搜索仅得到 `AppShell.tsx:53` 的一个 `SidebarProvider`、`:54` 的一个 `Sidebar`、`:108` 的一个 `SidebarInset`、`:122` 的一个 `SidebarTrigger`。
- `ProjectLayout.tsx:1-6` 只 import/render `Outlet`；没有 Sidebar imports、provider、fixed surface、gap、trigger 或第二套路由数组。
- 项目导航被合入 global SidebarContent。desktop 只有同一个 global fixed container/gap；mobile 由同一个 global Sidebar 的 Sheet 分支渲染，没有嵌套 provider 或第二 Sheet。
- `ResponsiveSidebarTrigger` 用 desktop `state` 与 mobile `openMobile` 计算 `isOpen`，生产代码会在 Open/Close 间动态切换 aria-label。desktop 行为测试真实点击 Close 后断言 Open；mobile 实现正确，但测试缺口见 R3-I2。
- 项目 section 数组包含与 `CONFIGURATION_SECTIONS` 一致的 12 个 canonical query：general、services、auth、smtp、oauth、storage、realtime、functions、database、pooler、network、secrets。Overview 仅一条；Runtime 仅 Logs/Backups，未与 section 重复。
- `^/projects/([^/]+)` + `projectId !== 'new'` 使 `/projects`、`/projects/new`、`/settings` 不渲染项目组；canonical project child routes渲染。当前生产逻辑正确。

## Round 3 全局旧轨扫描

```text
production SidebarProvider / Sidebar / gap owner          1 / 1 / 1
ProjectLayout Sidebar/provider/trigger/navigation array    0
LegacyConfigurationRedirect / compatibility route         0
十条旧 project child route                                0
旧 .sidebar/.panel/.button/.badge/.dialog/.progress CSS    0
raw feature <button>/<input>/<select>/<textarea>            0
raw <dialog>/<progress>                                     0
legacy className="alert error"                             0
delete-options / project-shell / project-nav                0
```

`sidebar-brand`/`sidebar-status` 仍有 global Sidebar 的当前生产消费者，不是无消费者旧 CSS。

## Round 3 验证证据

在 `44cbc59186b91debd7fec8a167f27fe64c08ae8d` 上只读运行：

```text
npm run test --workspace apps/web -- --run
PASS: 14 files, 84 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2228 modules transformed; build completed
WARN: dist/assets/index-EvISV62u.js 841.85 kB (gzip 261.63 kB)

go test ./...
PASS: all Manager, Provisioner, integration packages

git diff --check 9059455..44cbc59
PASS
```

构建后 `git status --short` 仍仅为允许写入的 `?? task-9-review.md`；Round 3 未修改任何实现、测试或提交。

---

# Round 4 只读复审

新增审计范围：`44cbc59..6efd8a5`；最终状态：`6efd8a5bb12ed319f873e361670f47684262a32d`（`fix: align project navigation with canonical sections`）。

裁决：**CHANGES REQUIRED / NOT APPROVED**

R3-I1 的 bare/invalid General active 已修；R3-I2 的真实 state owner、13 个 Project links（Overview + 12 sections）、Runtime links、desktop/mobile trigger 与三条非项目隔离测试已有实质修复。但全局 Main navigation 的 Projects 仍使用默认 prefix-active `NavLink`，在每个 project child route 自动产生第二个 `aria-current="page"`；新增“唯一”断言只在 Project navigation 内计数，仍是假阴性。故本轮仍有 1 个生产级 Important 和 1 个 bundle Minor。

## Critical

无。

## Important

### R4-I1. 项目子路由同时有 Main “Projects” 与当前 section 两个 `aria-current="page"`；唯一性测试错误地限制在子导航内

位置：

- `apps/web/src/app/AppShell.tsx:72-74`
- `apps/web/src/app/AppShell.tsx:90-94`
- `apps/web/src/app/AppShell.test.tsx:118-128`（最终行号）
- React Router NavLink 默认语义：`node_modules/react-router/dist/development/chunk-62JRHF6Z.mjs:10578-10633`

复现：

1. 访问 `/projects/bee/configuration?section=services`。
2. Main navigation 的 Projects 使用 `<NavLink to="/projects" />`，没有 `end`。React Router 的 `NavLink` 默认 `end=false`，因此 `/projects/bee/...` 对 `/projects` 是 active，并自动写入 `aria-current="page"`。
3. 新 section helper 同时正确把 Services Link 写成 `aria-current="page"`。
4. 整个 Sidebar 因而有两个“当前页面”：Main Projects 和 Project navigation Services。Overview、Logs、Backups 及其余 section 路由同样会与父级 Projects 重复。

影响：屏幕阅读器收到两个互相矛盾的 current page，直接违反本轮硬验收“aria-current 唯一”。父级 Projects 可以保持视觉 group-active，但不能继续以 `page` 声明自己就是当前 child 页面；应让 NavLink 对 child routes 不自动赋 `aria-current`（例如 exact/end 或显式 Link + 当前页语义分离）。

新增测试在 `within(projectNavigation)` 内过滤 `aria-current`，所以只证明 12 sections/Overview 之间唯一；它完全排除了 Main navigation，现状仍得到长度 1。换言之，该断言对本轮要求的**全 Sidebar 唯一性**仍是假阳性。应在 Sidebar 或 document 范围断言唯一，并分别覆盖 section、Overview 和 Runtime 代表路径。

只读补充验证 `matchPath({ path: '/projects', end: false }, '/projects/bee/configuration')` 返回 `/projects`；React Router 源码随后在 `isActive` 时自动设置 `aria-current`，因此这不是仅凭样式推测。

## Minor

### R4-M1. 生产 bundle 仍为 842.24 kB 单 chunk

Vite build 通过，但 `dist/assets/index-qU8GL6-z.js` 为 842.24 kB（gzip 261.71 kB），继续触发 >500 kB 警告。按要求记为 Minor。

## R3-I1 / R3-I2 重验

| Round 3 项 | Round 4 证据 | 裁决 |
|---|---|---|
| R3-I1 bare canonical active | `isActiveProjectLink` 将缺失 section 默认成 general，将不支持的 section normalize 成 general；bare、invalid 与 explicit OAuth 测试均断言唯一 child active。生产页面与 child nav 现在一致 | **原项已修复**，但全局唯一仍受 R4-I1 阻断 |
| R3-I2 测试假阳性/缺口 | desktop 明确 stub 1024px 并查询真实 `[data-slot=sidebar][data-state]`；Project nav 精确断言 Overview + 12 section 的 13 个 href 和 Set 唯一；Runtime 单独断言 Logs/Backups；mobile 断言真实 `sidebar-trigger` 变 `Close sidebar` 且 mobile Sheet open；`/projects`、`/projects/new`、`/settings` 均断言无项目组 | **大部分已修复**；`aria-current` 唯一断言仍因 scope 错误产生 R4-I1 假阴性 |

## 导航完整性复审

- Project links 正好 13 条：Overview + general/services/auth/smtp/oauth/storage/realtime/functions/database/pooler/network/secrets；每条 href 与 canonical query 一致，无重复。
- Runtime navigation 独立为 Logs、Backups 两条；未与 Overview 或 12 sections 重复。
- bare `/configuration` 与 invalid section 都映射 General；12 个 explicit supported sections 共用同一权威 helper，当前 section 计算正确。
- `/projects`、`/projects/new`、`/settings` 不渲染项目组；canonical project routes 才渲染。
- ProjectLayout 仍仅为 Outlet，没有第二套导航、Provider、Sidebar、fixed surface、gap 或 trigger。
- desktop/mobile 继续使用 AppShell 同一个 Sidebar；trigger 根据 `state/openMobile` 在 Open/Close 间变化。

## Round 4 全局旧轨扫描

```text
production SidebarProvider / Sidebar / gap owner          1 / 1 / 1
ProjectLayout Sidebar/provider/trigger/navigation array    0
LegacyConfigurationRedirect / compatibility route         0
十条旧 project child route                                0
旧 .sidebar/.panel/.button/.badge/.dialog/.progress CSS    0
raw feature <button>/<input>/<select>/<textarea>            0
raw <dialog>/<progress>                                     0
legacy className="alert error"                             0
delete-options / project-shell / project-nav                0
```

没有发现新旧双轨、兼容 fallback 或 raw-control 回流。

## Round 4 验证证据

在 `6efd8a5bb12ed319f873e361670f47684262a32d` 上只读运行：

```text
npm run test --workspace apps/web -- --run
PASS: 14 files, 89 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2228 modules transformed; build completed
WARN: dist/assets/index-qU8GL6-z.js 842.24 kB (gzip 261.71 kB)

go test ./...
PASS: all Manager, Provisioner, integration packages

git diff --check 44cbc59..6efd8a5
PASS
```

构建后 `git status --short` 仍仅为允许写入的 `?? task-9-review.md`；Round 4 未修改任何实现、测试或提交。

---

# Round 5/5 最终只读复审

新增审计范围：`6efd8a5..3675e1d`；最终状态：`3675e1d4ee05aee43b863a31831be19c72acbf63`（`fix: match projects navigation exactly`）。

裁决：**APPROVED**

没有 Critical 或 Important。R4-I1 已真实修复：Main Projects 使用精确 `NavLink end`，整个唯一 Sidebar 在 Projects list、Overview、bare Configuration、explicit section、Logs、Backups 各恰好一个 `aria-current="page"`。测试从 Sidebar 根节点统计 Main + Project + Runtime 全部链接，删除 `end` 会使所有项目 child case 立即得到两个 current links 并失败。唯一剩余项是生产 bundle >500 kB 的既有 Minor warning。

## Critical

无。

## Important

无。

## Minor

### R5-M1. 生产 bundle 为 842.21 kB 单 chunk

Vite build 通过，但 `dist/assets/index-OyEzpmUu.js` 为 842.21 kB（gzip 261.70 kB），继续触发 >500 kB warning。该性能债务不影响 Task 9 的单轨 UI、交互语义、a11y 或配置正确性，不阻断批准。

## R4-I1 最终裁决

**已修复。**

生产证据：

- `apps/web/src/app/AppShell.tsx:72` 的 Main Projects 现在是 `<NavLink to="/projects" end />`，其视觉 `isActive` 同样只接受 `location.pathname === '/projects'`。
- React Router exact match 只读验证：`end=true` 对 `/projects/bee/overview` 返回 false，对 `/projects` 返回 true。因此 project child route 不再给父级 Projects 自动注入 `aria-current`。
- Overview、12 个 Configuration sections、Logs、Backups 各自仍仅在自身 canonical route 显式设置 `aria-current="page"`。

测试有效性：

- `AppShell.test.tsx:111-127` 从 `[data-slot="sidebar"][data-state]` 根节点取得**全部** links，包含 Main、Project 与 Runtime，而不是再局限于 `within(projectNavigation)`。
- 覆盖 `/projects`、`/projects/bee/overview`、bare `/configuration`、explicit OAuth、Logs、Backups；每例断言 current link 数量严格为 1 且 accessible name 正确。
- 若删除生产 `end`，五个 project child cases 的 Main Projects 会重新 active，current link 数量从 1 变 2，因此测试必然失败；不是字符串或错误节点断言。
- 前一轮测试继续覆盖 invalid section -> General、12 个 canonical href、Runtime href、真实 desktop `data-state` owner、desktop Open/Close trigger、mobile trigger + open Sheet、`/projects`/`/projects/new`/`/settings` 项目组隔离。

## 最终架构与旧轨复核

```text
production SidebarProvider / Sidebar / gap owner          1 / 1 / 1
ProjectLayout Sidebar/provider/trigger/navigation array    0
LegacyConfigurationRedirect / compatibility route         0
十条旧 project child route                                0
旧 .sidebar/.panel/.button/.badge/.dialog/.progress CSS    0
raw feature <button>/<input>/<select>/<textarea>            0
raw <dialog>/<progress>                                     0
legacy className="alert error"                             0
delete-options / project-shell / project-nav                0
```

最终确认：

- AppShell 是唯一 Sidebar provider/fixed/gap owner；ProjectLayout 只渲染 Outlet。
- desktop 与 mobile 共用同一 Sidebar，mobile 走同一 primitive 的 Sheet 分支。
- 项目导航为 Overview + 12 canonical Configuration links；Runtime 仅 Logs/Backups，无 href 重复。
- bare/invalid/explicit section 的 page 与 nav active 一致；整个 Sidebar 仅一个 `aria-current`。
- `/projects`、`/projects/new`、`/settings` 不渲染项目组。
- 没有兼容 fallback、旧路由、第二 UI 轨、死旧 CSS 或 raw-control 回流。

## Round 5 验证证据

在 `3675e1d4ee05aee43b863a31831be19c72acbf63` 上只读运行：

```text
npm run test --workspace apps/web -- --run
PASS: 14 files, 95 tests

npm run lint --workspace apps/web
PASS: tsc --noEmit

npm run build --workspace apps/web
PASS: 2228 modules transformed; build completed
WARN: dist/assets/index-OyEzpmUu.js 842.21 kB (gzip 261.70 kB)

go test ./...
PASS: all Manager, Provisioner, integration packages

git diff --check 6efd8a5..3675e1d
PASS
```

构建后 `git status --short` 仍仅为允许写入的 `?? task-9-review.md`；Round 5 未修改任何实现、测试或提交。
