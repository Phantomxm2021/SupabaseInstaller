# Task 6 Review — CHANGES REQUIRED

审计范围：`fd653b1..c9975ed`。未审查 lockfile 的机械内容，仅核对了新增依赖的实际解析结果。没有发现 Critical；存在 Important，因此本轮不予 `APPROVED`。

## Important

- **Important — `apps/web/src/features/settings/ManagerSettingsPage.tsx:13`：Settings 的同 key 异构 queryFn 会轮换服务端 CSRF，却丢弃新 token，随后所有 mutation（包括 Sign out）可稳定失败。** `AuthenticatedShell` 为 `['session']` 注册的 queryFn 会把 GET `/api/session` 返回的 token 写入 `api/client.ts`；Settings 对相同 key 又注册了另一个 queryFn，并在第 16–18 行重新 GET session 后只返回两个 safe fields。Manager 的 GET session 会调用 `RefreshCSRF`，即服务端旧 token 立即失效；当缓存超过全局 5 秒 staleTime 后进入 Settings（或该 observer 触发 refetch），Settings 丢弃新 token，客户端仍发送旧 token，后续 DELETE/POST 收到 `INVALID_CSRF`。应只有一个 session fetch/query 定义负责同步 CSRF，Settings 通过 `select` 或纯展示 DTO 读取 `username`/`mustChangePassword`，且不得把 token 传给可见组件。`ManagerSettingsPage.test.tsx:13` 独立挂载页面，没有通过 `AuthenticatedShell` 的共享 cache，也没有在 refetch 后执行 mutation，因此完全漏掉了该回归。

- **Important — `apps/web/src/app/AppShell.tsx:35`：官方 Sidebar 被包回旧的固定 grid，且没有 `SidebarTrigger`，响应式导航在窄屏不可用。** 官方生成组件在 `<768px` 会把 Sidebar 渲染为默认关闭的 Sheet；当前 AppShell 没有导入或渲染 `SidebarTrigger`/等效 `toggleSidebar` 控件，所以触屏用户无法打开 Projects、Manager settings 或 Sign out。与此同时 `styles.css:91` 仍给外层 `.app-shell` 分配 `238px 1fr`，而官方 Sidebar 自己已生成 `16rem` gap；`styles.css:219` 在 768–820px 又把旧列缩成 72px，此时 Sidebar 仍是 256px desktop 版本，会覆盖大块主内容。应按官方 `SidebarProvider > Sidebar + main/SidebarInset` 组合布局，并在主内容中提供可聚焦的 trigger；至少加入 mobile/中间断点行为测试。官方结构参考：<https://ui.shadcn.com/docs/components/base/sidebar>。

- **Important — `apps/web/src/styles.css:277`：shadcn 的 surface token 覆盖了旧的文字 token，造成全站低对比度/不可读文本。** 旧 UI 把 `--muted` 当文字色（原值 `#939c97`），并在 `styles.css:65` 及大量页面说明、表单提示、topbar、project nav 中直接 `color: var(--muted)`；新 `.dark` 主题却按 shadcn 语义把同名 `--muted` 设为背景面色 `#1d211f`。`main.tsx:12` 强制添加 `.dark` 后，这些文字在 `#101211`/`#171a18` 背景上接近消失，违反可访问性和“dark neutral identity”验收。应把旧文字用途迁移到 `--muted-foreground`（或独立 legacy text token），并补充对暗色主题下关键文本对比度的验证。

- **Important — `apps/web/src/features/project/LifecycleActions.tsx:14`：删除成功流程没有 cancel configuration query，不满足要求的 cancel/remove detail+config 顺序。** 当前只 cancel `['project', id]`，随后直接 remove `['project-configuration', id]`。若 configuration 请求仍在飞行，其完成后可重新写回已删除项目的 cache；正确顺序应对 detail 和 configuration 都先 await cancel，再 remove 两者，之后 invalidate `['projects']` 并 replace-navigate。`LifecycleActions.test.tsx:13` 反而把缺少 `cancel:project-configuration/bee` 的序列固化为期望，同时它只测导出的 helper，没有驱动真实 mutation，未验证 API 成功/失败、Sonner success/error、`navigate(..., { replace: true })` 或它们之间的顺序。

- **Important — `apps/web/src/app/AppShell.tsx:41`：全局导航失去了 navigation landmark。** `aria-label="Main navigation"` 被放在 `SidebarMenu` 生成的 `<ul>` 上，AppShell 中没有 `<nav>`；屏幕阅读器的 landmark navigation 无法发现这组全局链接。现有 `AppShell.test.tsx:25` 只查询任意 Projects link，不能验证 landmark、当前项或键盘可达性。应以带名称的 `<nav>` 包裹 menu（并为 active route 提供可靠状态），加入 `getByRole('navigation', { name: ... })` 级别的测试。

## Minor

- **Minor — `apps/web/src/main.tsx:12`：应用强制暗色，但 Sonner theme 没有与该状态同步。** `components/ui/sonner.tsx:6` 从 `next-themes` 读取 theme，根节点却没有 `ThemeProvider`；默认得到 `system`。在浅色系统上，toast 会按浅色渲染在强制暗色应用之上。应提供一致的 ThemeProvider，或在固定暗色产品中明确给 Toaster 传 `theme="dark"`。

- **Minor — `apps/web/src/features/project/DeleteProjectDialog.tsx:41`：删除模式 radio group 的 `fieldset` 没有 `legend`。** 两个 radio 各自有 label，但辅助技术无法获得这组选择的组名；可用 visually-hidden legend 表达“Delete mode”。AlertDialog 本身的 Title/Description、focus primitive、两种模式及 exact-name 比较均保留。

## Verified

- `npm run test --workspace apps/web -- --run`：PASS，9 test files / 11 tests。
- `npm run build --workspace apps/web`：PASS，TypeScript `--noEmit` 与 Vite production build 均成功；仅有 658.97 kB chunk warning。
- `npm ls --workspace apps/web --depth=0 ...`：新增 Tailwind/shadcn/Base UI/Radix/Sonner 依赖均已安装并解析；lockfile 与 package manifest 一致。
- `components.json`、Tailwind Vite plugin、`@/*` 的 TS/Vite 双端 alias、`cn` helper 与生成的 base-nova 组件结构符合当前官方 Vite/manual setup：<https://ui.shadcn.com/docs/installation/vite>、<https://ui.shadcn.com/docs/installation/manual>。
- Global Sidebar 中没有 New Project；Projects 页仍保留 New project 主按钮；Manager settings 指向认证下的 React `/settings` 路由而非 `/api/session`。
- Settings 可见 JSX 仅使用 `username`、`mustChangePassword` 与静态 control-plane 文案；未发现 CSRF token 的 visible prop、日志或文本输出（但存在上述 token 同步失效）。
- Sign out 的 happy path 使用 DELETE `/api/session`、清空 CSRF/query cache 并 replace 到 `/login`（但会被上述 Settings CSRF 轮换问题破坏）。
- 删除 UI 保留 runtime-only / runtime-and-data 两种模式和大小写敏感 exact-name；成功/错误代码均调用 Sonner，detail/list query key 与当前页面一致，configuration key 与 Task 8 规定的 `['project-configuration', projectId]` 一致。
- `PREPARE_SUPABASE` 已从 web UI 源码移除。

---

# Task 6 Fix Round 1 Review — CHANGES REQUIRED

复审范围：`c9975ed..8b30ba4`。逐项回归上一轮 5 个 Important、2 个 Minor，并检查修复新增问题。没有发现 Critical；仍存在 Important，因此本轮不予 `APPROVED`。

## Important

- **Important — `apps/web/src/app/AppShell.tsx:37`：Sidebar 与 SidebarInset 仍不是 flex 布局中的兄弟，桌面主内容会被 fixed Sidebar 覆盖。** 修复删除了旧 `.app-shell` grid（`styles.css` 已无该规则），却保留 `SidebarProvider > div.app-shell > Sidebar + SidebarInset` 这一额外 wrapper。`SidebarProvider` 的 `display:flex` 现在只作用于单一 wrapper；wrapper 自身既不是 flex/grid，也没有宽度布局规则，因此 Sidebar 生成的 in-flow gap 与 `SidebarInset` 按普通 block 流纵向排列，不能为 fixed 256px Sidebar 在主内容左侧保留空间，主内容从 x=0 开始并被覆盖。应移除该 wrapper，让 `Sidebar` 和 `SidebarInset` 成为 `SidebarProvider` 的直接 flex children，或明确把 wrapper 设为 `display:flex; width:100%`。`AppShell.test.tsx:36` 也不能证明修复：它只 stub `matchMedia.matches`，但 `use-mobile.ts` 实际用 `window.innerWidth`（jsdom 默认 1024）计算状态；并且 trigger 在 desktop/mobile 都存在，测试既没进入移动 Sheet 分支，也没点击并验证 Sheet/navigation 可见。

- **Important — `apps/web/src/features/project/LifecycleActions.test.tsx:29`：声称覆盖 API/toast/replace/order 的测试仍是弱断言，未验证其中三项，也未验证完整顺序。** fetch stub 接受任意 URL/method/body 且从未检查请求；mock 的 `toast.success/error` 从未 import/expect；`Location` 只检查最终 pathname，不能区分 replace 与 push；记录数组只包含 cache helper 调用，不含 toast/navigate，因此跨层调用顺序即使错误测试仍会通过；也没有失败响应的 Sonner error 用例。生产实现 `LifecycleActions.tsx:38-44` 当前确实是正确 endpoint/body、cancel detail/config、remove detail/config、invalidate list、success/error toast、replace navigate，但 Task 6 明确要求“测试真实而非弱断言”，且 `task-6-report.md` 的“real API/toast/order coverage”陈述不成立。应让测试断言 DELETE URL/body、两类 toast、replace 语义，并把 cache/toast/navigate 纳入同一可观测顺序。

## Minor

- **Minor — `apps/web/src/app/AppShell.tsx:46`：Projects 只有 React Router 的语义 active，未接入 shadcn SidebarMenuButton 的视觉 active state。** `NavLink` 正确生成 `aria-current="page"`，landmark 也已修复；但 `SidebarMenuButton` 的 `isActive` 仍保持默认 false。生成组件的 active 样式依赖 `data-active`，而旧 `.sidebar nav a.active` 规则不会匹配生成 Sidebar（没有 `.sidebar` class）。因此当前项对屏幕阅读器可识别，但没有 shadcn active 高亮。应从 route match 传入 `isActive`，并测试 `data-active`/对应状态，而不只测试 `aria-current`。

- **Minor — `apps/web/src/app/AppShell.tsx:70`：toggle 按钮始终标为 “Open sidebar”。** 打开后它仍叫 Open，桌面 expanded 状态下也同样不准确。生成组件本来提供中性的 “Toggle Sidebar” 文本；应保留动态或中性 accessible name。当前测试只证明按钮存在。

## Round 1 findings resolved

- 共享 session/CSRF：已修复。`api/session.ts` 提供唯一 `['session']` queryFn，每次 GET 后同步新 CSRF；Settings 用 `select` 仅向组件暴露 `username` 与 `mustChangePassword`。新增 refetch→signout 测试真实检查了新 token header。
- Muted contrast：已修复。旧文字用法全部迁至 `--muted-foreground`；`var(--muted)` 只保留给 shadcn surface token。
- Delete cache implementation：已修复。实现依次 await cancel detail/config、remove detail/config、await invalidate list，再 toast 并 replace-navigate；query keys 与当前 detail/list 及 Task 8 configuration key 匹配。
- Navigation landmark：已修复为命名 `<nav>`，React Router 也提供 `aria-current`；视觉 active 仍见 Minor。
- Sonner theme：已修复为固定 `theme="dark"`。
- Delete mode group：已加入 sr-only `legend`。

## Round 1 verification

- `npm run test --workspace apps/web -- --run`：PASS，9 test files / 14 tests。
- `npm run lint --workspace apps/web`：PASS（TypeScript `--noEmit`）。
- `npm run build --workspace apps/web`：PASS，2099 modules；仅有 659.97 kB chunk warning。
- `git diff --check c9975ed..8b30ba4`：PASS。

---

# Task 6 Fix Round 2 Review — APPROVED

复审范围：`8b30ba4..856fdee`。Round 1 的 2 个 Important 与 2 个 Minor 均已关闭；未发现新的 Critical 或 Important。**APPROVED**。

## Minor

- **Minor — `apps/web/src/app/AppShell.tsx:47`：移动端选择导航目标后 Sheet 不会自动收起。** `SidebarMenuButton` 中的 Projects `NavLink` 以及第 63 行 Settings link 都没有调用 `setOpenMobile(false)`；在 Settings→Projects 或 Projects→Settings 的窄屏流程中，路由会改变，但 Sheet 仍覆盖新页面，用户还需点击 backdrop/按 Escape 关闭。`AppShell.test.tsx:44` 只验证 Sheet 能打开和导航可见，没有点击目的地并验证 Sheet 收起。这不阻断导航或可访问退出方式，因此不降为 Important。

## Round 2 findings resolved

- Sidebar wrapper/layout：已移除额外 `.app-shell`，`Sidebar` 与 `SidebarInset` 现在是 `SidebarProvider` 的两个直接 flex children；结构测试明确锁定该关系。
- Real mobile branch：测试将 `window.innerWidth` 设为 500，effect 后确认关闭 Sheet 中没有 navigation，点击 Open 后确认 named navigation 与 Projects link 可见。
- Delete full chain：测试真实断言 DELETE URL/method/body，按 `cancel detail → cancel config → remove detail → remove config → invalidate list → success toast → navigate` 校验完整 timeline，并通过 history back 证明 replace；失败路径断言 error toast 且不导航。
- Active state：Projects route 同时具备 React Router `aria-current` 与 shadcn `data-active`，实现语义和视觉 current state。
- Trigger label：通过 sidebar context 根据 desktop/mobile 的真实 open state 动态提供 Open/Close accessible name。
- `DeleteProjectDialog` 在确认时先关闭 dialog 再启动 mutation；成功/失败均由页面内状态与 Sonner 反馈，未破坏两种模式或 exact-name 约束。

## Round 2 regression verification

- 共享 session query 仍是唯一 `['session']` fetch 定义；Settings 继续通过 `select` 只接收 safe fields，CSRF refetch→signout header 测试通过。
- 删除实现仍严格 cancel/remove detail+configuration、invalidate list、success/error Sonner、replace navigation；query keys 未漂移。
- Global Sidebar 仍无 New Project；Settings 仍为认证 React `/settings` route；Sign out happy path 可用。
- Muted contrast、固定暗色 Toaster、navigation landmark、delete-mode legend 与 `PREPARE_SUPABASE` 移除均保持。

## Round 2 verification

- `npm run test --workspace apps/web -- --run`：PASS，9 test files / 17 tests。
- `npm run lint --workspace apps/web`：PASS（TypeScript `--noEmit`）。
- `npm run build --workspace apps/web`：PASS，2099 modules；仅有 660.12 kB chunk warning。
- `git diff --check 8b30ba4..856fdee`：PASS。
