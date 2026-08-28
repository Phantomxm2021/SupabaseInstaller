# Single-Source Runtime Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Manager 的项目配置收敛为单一事实来源：用户在 Dashboard 修改配置后，Manager 直接持久化该项目的 canonical configuration，生成 Supabase 官方 Compose/env 文件并重建/重启受影响服务。移除 revision、last-good、candidate、lease、fence、双份运行时文件和旧迁移逻辑，避免配置冲突、STALE_CONFIG_REVISION、回滚污染及“界面成功但 Docker 没变化”。

**Architecture:** Manager SQLite 保存项目身份、一个项目配置快照和加密 secrets；Provisioner 只接收完整 canonical configuration，生成运行时文件并执行 Docker Compose；`.manager-runtime` 只保存当前生成的文件，不再保存 `project.json`、generation/current symlink 或候选版本。一次项目只能有一个配置应用操作，由 Manager 的应用锁串行执行。配置提交失败时保留数据库中的用户配置并返回脱敏后的精确 Compose/health 错误，Retry 重放同一 canonical configuration。

**Tech Stack:** Go (Manager/Provisioner), SQLite, Docker Compose, React 19 + TypeScript + Vite, Vitest, Go test.

**Spec:** 本文件的 Architecture、Global Constraints 和 Acceptance Criteria 即本次对话确认的实现契约。

## Current implementation status (2026-08-28)

The canonical read/write path, revision-free normal PATCH contract, direct
apply orchestration, concrete Provisioner diagnostics, and official database
bootstrap file permissions are implemented and covered by tests. The old
revision/lease tables and generation directory remain only as migration and
password-rotation compatibility storage in this rollout; they are not read by
normal Dashboard applies. The remaining unchecked tasks are the follow-up
removal migration and fixed-artifact filesystem cutover, which require an
explicit maintenance window because they change on-disk project layout.

## Global Constraints

- Manager SQLite 是唯一配置源；Supabase host 只读取 Provisioner 生成的 `.manager-runtime/docker-compose.yml`、`.env` 和 `.env.functions`。
- 不再维护 desired/last-good/candidate 三套配置，不再用 revision/fence/idempotency 判断配置新旧。
- Dashboard PATCH 只提交 `{ value }`；API 不再要求 `expectedRevision`，不返回 `STALE_CONFIG_REVISION` 或 `CONFIGURATION_CONFLICT`。
- 运行时配置更新使用 `docker compose up -d --remove-orphans`，不使用 `--build`；只有镜像/Dockerfile 变更才显式 build。
- 关闭服务只移除该服务容器，保留数据库和其它持久卷；打开服务重新生成 Compose 并启动该服务。
- 失败信息必须包含阶段、服务名、退出码及经过脱敏的 Compose/health 输出；禁止只显示 “runtime reconciliation failed”。
- 不删除用户的 `volumes/db/data`。迁移或清理只处理 Manager 自己创建的数据库行和 `.manager-runtime` 产物。
- `/auth/users`、`/auth/oauth-apps` 继续代理运行中 GoTrue；它们不是 Manager 配置表，也不改写用户数据。
- 保留用户现有的 UI 视觉/交互契约（双侧栏、固定二级侧栏、Provider 抽屉动画、Users/OAuth/Email 页面布局）；本计划只调整数据链路和错误处理，不重新设计页面。

---

## Task 1: Freeze the new contract and map every legacy path

**Files:** `internal/contracts/`, `apps/manager/internal/httpapi/configuration.go`, `apps/manager/internal/project/configuration_service.go`, `apps/manager/internal/configuration/orchestrator.go`, `apps/provisioner/internal/runtime/`, `apps/web/src/features/project/configuration/useConfigurationMutation.ts`.

- [ ] Add contract tests describing one canonical configuration read, PATCH `{value}`, and one apply operation per project.
- [ ] Record the exact JSON shape passed from Manager to Provisioner (complete typed configuration, transient decrypted secrets, project identity, operation ID); exclude revision, expected revision, next revision, fence and idempotency fields.
- [ ] Mark every call site of `GetDesiredConfiguration`, `last_good_revision`, `AdmitConfiguration`, `MarkConfigurationGood`, stale-revision errors and candidate restoration for removal in later tasks.
- [ ] Add a repository check that fails if the removed protocol fields or stale/conflict error codes reappear in production code.

**Verification:** `go test ./internal/contracts/... ./apps/manager/internal/httpapi/... ./apps/manager/internal/project/... ./apps/provisioner/internal/runtime/... -count=1` and `rg -n "ExpectedRevision|NextRevision|last_good_revision|STALE_CONFIG_REVISION|CONFIGURATION_CONFLICT" internal apps`.

## Task 2: Replace revisioned configuration tables with one canonical row

**Files:** `apps/manager/internal/store/migrations/014_single_source_configuration.sql`, `apps/manager/internal/store/configuration.go`, `apps/manager/internal/store/project.go`, `apps/manager/internal/store/*_test.go`.

- [ ] Add a migration that creates `project_configuration(project_id PRIMARY KEY, config_json, updated_at)` and a small `project_configuration_keys(project_id, kind, value)` table only for uniqueness/index checks; do not copy the full configuration into another table.
- [ ] Backfill one canonical row per project from the newest valid aggregate configuration, preferring the current project configuration and then the last known valid row during migration; record a migration error instead of silently inventing defaults.
- [ ] Keep encrypted current secrets in `project_secrets`; remove version/snapshot tables and their foreign keys after backfill.
- [ ] Make `GetConfiguration` and `GetDesiredConfiguration` a single `GetCanonicalConfiguration` query. Return the same value that the UI displays and the Provisioner applies.
- [ ] Make `SaveCanonicalConfiguration` a transaction: validate typed sections, update the one row, refresh `project_configuration_keys`, and release the application lock only after the row is committed.
- [ ] Remove independent config authority from `projects` (`domain`, `site_url`, `supabase_version`, `preset`, `services_json`, `config_revision`, `last_good_revision`); retain identity/status fields and runtime health metadata only.
- [ ] Add tests for first save, update, secret rotation, duplicate domain/port rejection, rollback-free persistence, and concurrent writers.

**Verification:** `go test ./apps/manager/internal/store/... -count=1`; inspect a migrated database and assert exactly one `project_configuration` row per project.

## Task 3: Simplify Manager PATCH and operation orchestration

**Files:** `apps/manager/internal/httpapi/configuration.go`, `apps/manager/internal/project/configuration_service.go`, `apps/manager/internal/configuration/orchestrator.go`, `apps/manager/internal/store/operations.go` and tests.

- [ ] Change PATCH `/api/projects/{projectId}/configuration/{section}` to accept `{ value }` only; remove expected-revision parsing and conflict branches.
- [ ] Merge the section into the canonical row, validate it, persist it, and enqueue one `APPLY_CONFIGURATION` operation containing the resulting complete configuration.
- [ ] Add a per-project application lock in process plus a database guard so repeated clicks coalesce onto the active operation instead of creating many `RUNNING` operations stuck at 70%.
- [ ] Make operation progress explicit: `PERSIST_CONFIGURATION`, `RENDER_RUNTIME`, `COMPOSE_UP`, `VERIFY_SERVICES`, `SUCCEEDED`/`FAILED`; every transition writes an event with a redacted detail string.
- [ ] On apply failure, leave canonical configuration intact, mark the operation failed, and expose the exact error to the API/UI. Do not restore a candidate or “last-good” configuration.
- [ ] Retry loads canonical configuration from the database and starts a fresh apply operation; it never reuses a stale revision or operation payload.
- [ ] Remove compensation/lease/reservation code paths whose only purpose was revision rollback.

**Verification:** Manager tests cover PATCH, duplicate submit, failure persistence, retry, and exact error response. Run `go test ./apps/manager/internal/httpapi/... ./apps/manager/internal/project/... ./apps/manager/internal/configuration/... -count=1`.

## Task 4: Replace Provisioner revision protocol with a direct apply API

**Files:** `internal/contracts/`, `apps/provisioner/internal/httpapi/`, `apps/provisioner/internal/runtime/reconcile.go`, `apps/provisioner/internal/runtime/*_test.go`.

- [ ] Define `ApplyProjectConfigurationRequest` with project ID, slug, operation ID, complete configuration and transient secrets; remove revision/fence/idempotency members.
- [ ] Validate project identity and configuration, then call the renderer directly. Do not read `project.json` or compare request revisions.
- [ ] Return structured failures (`phase`, `service`, `exit_code`, `message`, `logs`) so Manager can display the actual cause while secrets remain redacted.
- [ ] Make the handler idempotent by operation ID only: a repeated request for the same operation returns its recorded result, without treating a newer/older numeric revision as an error.
- [ ] Add tests for successful apply, invalid config, Docker failure, health timeout, repeated operation ID, and disabled-service transitions.

**Verification:** `go test ./apps/provisioner/internal/httpapi/... ./apps/provisioner/internal/runtime/... -count=1`.

## Task 5: Collapse runtime filesystem to one atomic artifact set

**Files:** `apps/provisioner/internal/projectfs/root.go`, `apps/provisioner/internal/render/render.go`, `apps/provisioner/internal/render/environment.go`, `apps/provisioner/internal/compose/runner.go`, related tests.

- [ ] Remove `Metadata`, `project.json` read/write, generation directories, `current` symlink, candidate/quarantine cleanup and rotation journals from the active path.
- [ ] Define fixed paths under `<project>/.manager-runtime/`: `docker-compose.yml`, `.env`, `.env.functions`, `templates/`; create the directory with owner/mode required by Docker.
- [ ] Render into an operation-specific temporary directory under `.manager-runtime`, run syntax/secret/file checks, then atomically rename each file (or the complete directory) into place.
- [ ] Update Compose runner to use the fixed files and stable `--project-directory <project root>` and `--project-name supabase-manager-<slug>` arguments.
- [ ] Reconcile with `docker compose config --quiet` followed by `docker compose up -d --remove-orphans`; recreate only services whose rendered configuration changed, while preserving volumes.
- [ ] Keep official template versions and service filtering in `internal/templates/self-hosted-v0.8.0`; do not introduce a second hand-written Compose template.
- [ ] Add renderer/runner tests asserting fixed paths, generated `API_EXTERNAL_URL`/SMTP/auth values, no `--build`, and no accidental `.manager-runtime/current/.env` path.

**Verification:** `go test ./apps/provisioner/internal/projectfs/... ./apps/provisioner/internal/render/... ./apps/provisioner/internal/compose/... -count=1`; run `docker compose config --quiet` against a generated Standard project.

## Task 6: Remove legacy startup migrations and dual-track schema code

**Files:** `apps/manager/internal/store/migrations/002_project_configuration.sql` through `013_operation_compensation.sql`, `apps/manager/cmd/manager/main.go`, `apps/manager/internal/store/legacy*`, `apps/provisioner/internal/projectfs/*`, tests.

- [ ] Make migration `014_single_source_configuration.sql` the only supported upgrade path; mark `002`–`013` as historical and stop executing their compatibility repair routines on every Manager boot.
- [ ] Remove `ResetLegacyAuthConfigurations`, `MigrateFailedPostgreSQL15Configurations` and similar startup mutations; provide one explicit offline migration command for existing installations.
- [ ] Remove project metadata repair that recreates missing `project.json`, generations, stale candidates or legacy env names.
- [ ] Remove revision leases, configuration reservations, operation configuration snapshots, operation secret snapshots, compensation phases and fence columns after data backfill.
- [ ] Delete stale tests and fixtures that assert rollback/candidate behavior; replace them with canonical-save/apply tests.
- [ ] Add a migration dry-run that reports projects requiring manual review and never deletes runtime volumes.

**Verification:** Start Manager on a migrated copy and assert no repair/migration loops in logs; `go test ./apps/manager/... ./apps/provisioner/... -count=1`.

## Task 7: Align Dashboard data flow and error UX without changing the approved layout

**Files:** `apps/web/src/features/project/configuration/useConfigurationMutation.ts`, `apps/web/src/features/authentication/AuthenticationWorkspace.tsx`, configuration pages/components, `apps/web/src/features/authentication/UsersPage.tsx`, `OAuthAppsPage.tsx`, operation hooks/tests.

- [ ] Remove `expectedRevision` from mutation state and PATCH payload; invalidate the canonical configuration query after success.
- [ ] Render the canonical value immediately after save; show apply progress separately so “saved” and “Docker applied” are not conflated.
- [ ] Replace generic error toasts with the API’s phase/service/message/log detail, preserving the existing dark Supabase-style layout, fixed sidebars, provider drawer animation, and right-aligned row controls.
- [ ] Keep Retry and add Delete only for failed project creation as previously requested; Retry must call the simplified apply endpoint.
- [ ] Remove UI copy for candidate, last-good, stale revision, configuration conflict and queued rollback states.
- [ ] Verify Users and OAuth Apps still call GoTrue proxy endpoints and are unaffected by the config rewrite.
- [ ] Add Vitest tests for PATCH payload, success invalidation, exact error rendering, retry, and Users/OAuth request paths.

**Verification:** `cd apps/web && npm test -- --run`; `npm run build`.

## Task 8: End-to-end Docker acceptance on a Standard project

**Files:** `tests/e2e/` (new), `deploy/docker-compose.yml`, test fixtures and scripts.

- [ ] Start Manager and Provisioner with Docker Desktop using `deploy/docker-compose.yml` and `deploy/.env`; build product images only once before the test.
- [ ] Create a Standard project and assert Manager DB contains one canonical configuration row and no candidate/last-good rows.
- [ ] Assert generated paths are `<project>/.manager-runtime/docker-compose.yml`, `.env`, `.env.functions`; assert no `project.json` or `current` symlink is required.
- [ ] Change Site URL, SMTP, Rate Limits and one provider/service flag; verify the canonical row changes and only the expected project containers receive new `StartedAt`/config hashes.
- [ ] Disable and re-enable a service; verify the container is removed/recreated while volumes remain.
- [ ] Inject an invalid Compose value and assert the operation ends `FAILED` with the exact phase/service/error visible in Dashboard and Manager/Provisioner logs.
- [ ] Retry the failed operation and assert no stale-revision/conflict error and a successful health check.
- [ ] Query Users and OAuth Apps and assert requests reach the running GoTrue service.
- [ ] Delete a project and assert its Compose project and Manager metadata are removed, while unrelated projects and host Docker resources remain intact.

**Verification:** run the scripted suite plus `docker compose -f deploy/docker-compose.yml --env-file deploy/.env ps` and archive redacted logs/artifacts.

## Task 9: Document source-of-truth, deployment and recovery operations

**Files:** `README.md`, `docs/operations/project-configuration.md` (new), `docs/operations/deployment.md`, `docs/operations/migrations.md` (new), `docs/operations/nginx.md` if present.

- [ ] Document that Dashboard writes Manager SQLite, Provisioner renders `.manager-runtime`, and Supabase containers read only generated env/Compose files.
- [ ] Document project-level paths, fixed host ports (API gateway 8000, Studio 8001 unless explicitly configured), Nginx/Cloudflare routing, and when a product image rebuild is required.
- [ ] Provide an upgrade runbook: stop Manager/Provisioner, back up `manager.db` and project volumes, run migration dry-run, apply migration, regenerate each runtime, then start services; never delete `volumes/db/data` during configuration repair.
- [ ] Provide diagnostics commands for Manager/Provisioner logs, operation events, generated Compose validation and container health, with commands that select containers by Compose labels rather than guessed names.
- [ ] Document exact failure phases and retry/delete behavior so operators no longer have to infer whether Docker was touched.

## Acceptance Criteria

- [ ] A configuration PATCH changes one canonical DB row and eventually the corresponding generated runtime files; there is no second desired/last-good/candidate value.
- [ ] A successful apply visibly recreates/restarts the required Supabase service containers without `docker compose build`.
- [ ] Any failure is surfaced with a concrete phase/service/error and remains retryable without revision conflicts.
- [ ] Standard project creation succeeds with official PostgreSQL bootstrap, auth, realtime, rest and storage services healthy; no missing `graphql_public`, auth-admin password or `_supabase` database caused by Manager-generated config.
- [ ] Users/OAuth Apps continue to operate through GoTrue; email/provider pages retain the approved screenshot-aligned UI.
- [ ] `go test ./apps/manager/... ./apps/provisioner/... -count=1`, `cd apps/web && npm test -- --run`, and `npm run build` pass.
- [ ] `git diff --check` passes and `rg` finds no active stale revision protocol or legacy startup repair call.

## Rollout and rollback safety

- [ ] Before implementation, create a timestamped backup of Manager SQLite and every project’s `volumes/` directory.
- [ ] Roll out schema/runtime changes behind a maintenance window; apply the migration before starting the new Manager/Provisioner images.
- [ ] If the migration dry-run reports an ambiguous project, leave that project untouched and stop the rollout with its ID and reason.
- [ ] Rollback means restoring the Manager image and database backup together; do not mix old revision-aware binaries with the new single-source schema.
