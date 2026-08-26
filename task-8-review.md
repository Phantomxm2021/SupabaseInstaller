# Task 8 Review — NOT APPROVED

Review scope: `e000d25..a1c1969`, checked against the Task 8 plan, approved configuration design, PRD, final Task 7 ruling, backend contracts/endpoints, and `task-8-report.md`. The report was not treated as evidence.

## Critical

None.

## Important

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:50-55,117-123,132` and `apps/manager/internal/httpapi/configuration.go:71-75,99-123`: OAuth saving is wired to the wrong endpoint and payload, so every OAuth update is rejected before it can queue an operation.** The page sends the complete provider map to `PATCH /configuration/oauth`; the backend treats that route as a single-provider patch, decodes `value` as one `OAuthProviderConfig`, and obtains an empty `{provider}` path value. The required wire operation is one `PATCH /configuration/oauth/{provider}` with one provider value. The only page test never saves OAuth, so the regression is invisible.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:108-123,129,133-137`: section-local service switches edit `services.*` but the Save action serializes only the current non-services section, making the advertised enable/disable flows non-functional.** Auth and Direct DB changes are rejected because the saved Auth/Database value drifts from the unchanged server-side `services.auth`/`services.directDb`; Storage, Realtime, Functions, and Supavisor toggles are simply omitted and do not persist. The same service booleans also appear in Services, so users are given two controls with different effective persistence. This breaks the PRD's post-install OFF→ON flows and the single-authority section contract.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:43,146-152`: the implementation is not typed RHF/Zod configuration editing.** The sole resolver is `z.object({}).passthrough()`, every section shares one redacted-response form rather than owning an update schema, update actions are forced through `any`, and only text/number fields render errors. Invalid URLs, dependency changes, SMTP/OAuth/Storage/Functions secret actions, and conditional fields therefore pass the client confirmation unvalidated. This is exactly the generic pass-through boundary Task 8 prohibited.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:128-130,133-138`: multiple required typed fields and dependency behaviors are missing or dead.** Phone Auth has no Twilio `accountSid`/`messageServiceSid`/`verifySid`, MessageBird `originator`, or Textlocal `sender`, so enabling any provider cannot satisfy Manager validation. Studio is permanently disabled instead of being optional with postgres-meta closure; Logs/Vector and imgproxy/Storage are independent switches with no closure. Database extensions are presented as editable even though the current pinned client schema rejects every non-empty extension. The twelve tabs are therefore not complete typed sections.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:132`: generated OAuth callbacks are wrong.** The UI derives them from Site URL and appends `?provider=<name>`, while the approved PRD callback is the project public URL ending exactly in `/auth/v1/callback`; the existing wizard also demonstrates the no-query form. Copying the displayed value into a provider can break OAuth login.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:133,117-125`: object-storage transitions do not implement retain/remove semantics.** Switching a configured S3 backend to Local clears the visible strings but leaves `secretAccessKeySet` (and possibly `forcePathStyle`) set and resets the action to empty; normalization converts that configured marker back to `retain`, which Manager rejects for Local. Switching S3 to R2 also leaves the old endpoint, which Manager rejects because R2 derives it. A supported backend change can therefore not be saved.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:77-83,108-115`: dirty state and update preview are not truthful.** Save remains enabled for pristine sections and invents the section label as a change; `dirtyLabels` walks every dirty section instead of the section being submitted; affected services are a fixed per-tab list rather than calculated from changed fields; and the preview never distinguishes restart from recreate, only saying either “may be required.” It can list unsaved changes that are not in the request and claim impacts unrelated to the actual patch.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:65,139-142`: API/Secrets and rotation do not complete their operation contracts.** Project URL and Anon Key are placeholder prose rather than actual values, revealed values have no Copy action, reveal failures are not caught/rendered, and the database-rotation success handler discards the returned `operationId` and immediately invalidates configuration instead of opening the persistent `OperationPanel` and refetching after terminal success. The dialog warning and password re-entry exist, but the durable rotation cannot be monitored and failure/rollback is hidden.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:135,149` and `apps/manager/internal/project/configuration_service.go:272-277`: deleting a configured Functions variable row does not send a remove action for its encrypted secret.** The UI drops the row outright; Manager generates mutations only for variables still present in the incoming slice. The configuration loses the variable while the encrypted `functions.<name>` secret remains orphaned. Explicit Retain/Replace/Remove exists inside a retained row, but the adjacent destructive row action bypasses it.

- **Important — `apps/web/src/features/project/ConfigurationPage.tsx:127-149`: the forms fail the explicit accessibility acceptance requirement.** Switches, Select triggers, secret inputs, read-only inputs, and password confirmation inputs generally have visual `FieldLabel` text but no `id`/`htmlFor` or `aria-label` association. Repeated OAuth “Enabled” switches are not named by provider, and reveal/rotation password inputs rely on placeholders. Keyboard-operable shadcn primitives are present, but accessible names and field relationships are not.

- **Important — `apps/web/src/features/project/ProjectLayout.tsx:4-13` and `apps/web/src/app/router.tsx:46-69`: routing redirects do establish a single workspace, but the required first-class Configuration navigation item is absent and existing Logs/Backups entries were removed.** Logs is reachable only by typing its legacy URL, while the plan required routing the existing navigation into the relevant workspace section. Because all navigation targets share the same pathname and differ only by query, the current `NavLink` construction also cannot reliably identify one section as the active project item.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:7-15` and `apps/manager/internal/httpapi/configuration_test.go:33-65`: Task 8 has no behavioral regression coverage.** The only page test stubs one GET and checks that Database renders; it never covers redaction badges, retain/replace/remove payloads, OAuth/SMTP endpoints, conditional schemas, dirty state, 409 preservation/reload, preview impact, operation terminal behavior/refetch, four reveal kinds, timeout/section clearing, rotation, routing, or accessibility. The backend additions test pure merge helpers only, not real subsection handlers plus `QueuePatch`, and never assert that a configured target with empty action is rejected. Passing 46 tests therefore does not substantiate the Task 8 report.

## Minor

- **Minor — `task-8-report.md:21`: the claimed `git diff --check` result is false.** `git diff --check e000d25..a1c1969` reports `task-8-report.md:21: new blank line at EOF`.

## Verified partial behavior

- `apps/manager/internal/httpapi/configuration.go:199-233` correctly marks only server-merged, configured SMTP/OAuth/Phone siblings as internal `retain`; it skips the owned SMTP/provider leaf. `apps/manager/internal/project/configuration_service.go:141-190` still rejects a configured owned leaf whose incoming action is empty. The carried Task 7 sibling fix is logically sound, although its committed tests are not end-to-end.
- `apps/web/src/app/router.tsx:46-69` redirects legacy configuration routes into one `/configuration?section=...` workspace; no second installed-project form implementation remains.
- `apps/web/src/features/project/ConfigurationPage.tsx:42,61` uses the required configuration query key and invalidates configuration and project detail after a normal configuration operation succeeds. The 409 handler preserves the dirty form until Reload is chosen.
- `apps/web/src/features/project/ConfigurationPage.tsx:139-142` requests all four reveal kinds with password re-entry, stores plaintext only in component state, and schedules timeout clearing; leaving the section unmounts that state. The backend reveal response has `Cache-Control: no-store`. Copy and error behavior remain missing as noted above.

## Verification

- `npm test --workspace apps/web -- --run`: PASS, 11 files / 46 tests.
- `npm run build --workspace apps/web`: PASS; existing 810.19 kB chunk warning.
- `GOCACHE=/tmp/supabase-installer-task8-audit-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/project`: PASS.
- `git diff --check e000d25..a1c1969`: FAIL at `task-8-report.md:21`.

Verdict: **NOT APPROVED**. There are no Critical findings, but the installed-project configuration workspace has multiple load-bearing Important failures and the report materially overstates implementation and test coverage.

---

# Task 8 Fix Round 1 Review — NOT APPROVED

Round 1 scope: `a1c1969..203ec9d`. Every prior Important and Minor was rechecked against the production wire DTOs, section endpoints, Manager validation, secret mutation path, navigation, operation flow, and unchanged tests. The appended implementation report was not accepted as evidence.

## Critical

None.

## Important

- **Important — `apps/web/src/features/project/configuration/ConfigurationPage.tsx:44-47`, `AuthSection.tsx:11-18`, `OAuthSection.tsx:12-14`, and `DatabaseSection.tsx:8-9`: three sections crash on a normal redacted Manager response.** Go marks `auth.redirectUrls`, `auth.oauth`, and `database.extensions` `omitempty`, and the default installed configuration leaves all three nil. The page casts that response to non-optional create types; Authentication calls `auth.redirectUrls.join`, OAuth indexes `initial[provider]`, and Database calls `db.extensions.join`. The only page test fabricates the TypeScript create default containing `[]`/`{}`, so it cannot reproduce the real response. Authentication, OAuth, and Database are therefore unusable for a default real project.

- **Important — `apps/web/src/features/project/configuration/schema.ts:10-33`, `useConfigurationMutation.ts:30-42`, and `ConfigurationPage.tsx:36`: the new section resolvers are still not parity schemas and Manager field errors are discarded.** General accepts any non-empty Domain; SMTP lacks enabled-field, email, and secret-action conditions; Auth lacks Phone provider/required-field/secret and global-signup constraints; OAuth ignores its provider parameter and special URL fields; Storage does not validate endpoint URL or reject remove on an object backend; Functions lacks the environment-name/reserved-name and configured-secret rules. When Manager correctly returns `APIError.fields`, the mutation hook only emits a toast and has no form reference with which to attach them. This remains materially short of typed RHF/Zod sections with field-local authoritative errors.

- **Important — `apps/web/src/features/project/configuration/OAuthSection.tsx:17-22`: an enabled OAuth provider cannot be disabled.** Toggling it off makes `value.enabled` false, which removes the entire credential block including the only submit button. No PATCH can be sent for the disabled state. The provider endpoint itself is now correct, but the core ON→OFF operation is impossible.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:8-14`: Services still does not expose all supported post-install switches.** Studio, PostgREST, and postgres-meta remain permanently locked and described as required, although only PostgreSQL is globally mandatory and Studio should be optional with postgres-meta dependency closure. The new backend synchronization at `apps/manager/internal/project/configuration_service.go:78-85` correctly makes Services authoritative for Auth and Direct DB, but it does not justify locking these other supported services.

- **Important — `apps/web/src/features/project/configuration/types.ts:34-46` and `ConfigurationPage.tsx:39,47`: the update preview still misstates runtime behavior and affected services.** SMTP, OAuth, Storage, Functions environment, Auth, and Realtime configuration changes are all labelled “Service restart required,” while the approved flows and Provisioner use recreate for rendered configuration changes. General omits Studio even though Provisioner marks Auth, Studio, and gateway affected; Services impact is always “recreate” without distinguishing start/stop; the helper ignores its actual `value`. Dirty-only buttons and per-section labels are fixed, but affected/restart/recreate preview is not truthful.

- **Important — `apps/manager/internal/httpapi/configuration.go:49-76`, `apps/web/src/features/project/configuration/ConfigurationPage.tsx:29,34,47`, and `SecretsSection.tsx:8-15`: the fourth reveal kind, `anonKey`, now bypasses the required reveal boundary and is persisted in TanStack Query cache.** Every configuration GET decrypts Anon Key without password re-entry, returns it without `Cache-Control: no-store`, and `useQuery` retains it in `['project-configuration', projectId]`; the Secrets section simultaneously still offers the four-kind recent-auth reveal. This violates the direct four-kind reauth/no-cache requirement and makes the normal redacted configuration projection contain secret plaintext. Project URL, copy controls, reveal error rendering, memory timeout, section unmount clearing, and rotation operation handoff are otherwise implemented.

- **Important — `apps/web/src/features/project/configuration/GeneralSection.tsx:13-15`: the General section still displays and copies the wrong OAuth callback.** It derives callback from Site URL; the provider cards correctly use the Project URL derived from Domain. The same workspace therefore presents two conflicting callback authorities, one of which can be copied into provider configuration.

- **Important — `apps/web/src/features/project/configuration/FunctionsSection.tsx:10-12`: “Remove variable” does not remove a configured variable.** On the first click it leaves the row in the payload and only changes its secret action to `remove`; Manager deletes the encrypted secret and persists the same variable with `valueSet:false`. The user must refetch and click Remove a second time to remove the row. The previous orphaned-secret defect is closed, but the claimed configured-variable deletion remains an incomplete two-operation flow.

- **Important — `apps/web/src/features/project/ProjectLayout.tsx:4-14`: navigation remains semantically broken despite adding Configuration and query-aware active logic.** Services and Logs point to the identical `configuration?section=services`, so both are active simultaneously. Backups now points to General configuration rather than its existing `/backups` module/placeholder, and the prior Project Settings entry has disappeared. Redirects still establish one configuration workspace, but this is not a correct preservation/routing of existing project navigation.

- **Important — `apps/web/src/features/project/configuration/AuthSection.tsx:12-18`, `OAuthSection.tsx:22`, `FunctionsSection.tsx:12`, `NetworkSection.tsx:9`, and `fields.tsx:15-44`: the section rewrite does not follow the required shadcn form composition and only partially fixes accessibility.** It replaces generated shadcn Select/Input/Button/Textarea patterns with raw `<select>`, `<input>`, `<button>`, and `<textarea>` controls throughout. Basic label associations are improved, but Toggle/Select/schema errors are not rendered or described, server field errors cannot attach, and repeated reveal buttons are named only “Reveal” rather than by secret kind. The explicit shadcn and component-level accessibility acceptance remains unmet.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:7-15` and `apps/manager/internal/httpapi/configuration_test.go:33-65`: Round 1 changes production behavior across 24 files without changing a single test file.** There is still no mutation test for the now-per-provider endpoint, OAuth disable, real `omitempty` responses, section schemas, Manager field errors, service synchronization, Storage removal, preview impact, 409 Reload, reveal cache/timeout/copy, rotation operation, navigation, or accessibility. The backend validation and secret-response changes likewise have no new API/service regression tests. The unchanged 46-test pass cannot substantiate any Round 1 correction claim.

## Minor

- **Minor — `apps/web/src/features/project/configuration/SecretsSection.tsx:17`: Round 1 again claims `git diff --check` passed, but the exact range fails with `new blank line at EOF`.** The prior report-file whitespace defect was removed, then replaced with this one.

## Round 1 disposition of prior findings

- **Closed:** the OAuth client now targets `/configuration/oauth/{provider}`; service enablement has one UI owner and Manager synchronizes Auth/Direct DB projections; the generic pass-through resolver was removed; Phone provider fields exist; provider-card callbacks use Domain without a query; Local Storage transitions send an explicit remove accepted by Manager; pristine Save buttons and per-section dirty labels are fixed; Project URL/copy/reveal errors/rotation operation handoff exist; configured Functions secret removal no longer leaves an encrypted orphan; most basic labels have associations; a first-class Configuration route and exact query matching were added.
- **Still load-bearing or regressed:** exact redacted DTO handling, conditional schemas and field errors, OAuth disable, complete Services switches, truthful preview, four-kind no-cache reveal, one-step Functions deletion, navigation semantics, shadcn/accessibility, and behavioral tests, as detailed above.
- **Unchanged and still sound:** the Task 7 SMTP/OAuth sibling merge marks only untouched configured siblings as internal retain and still rejects an empty action on the owned target.

## Round 1 verification

- `npm test --workspace apps/web -- --run`: PASS, 11 files / 46 tests (unchanged).
- `npm run build --workspace apps/web`: PASS; 817.89 kB chunk warning.
- `GOCACHE=/tmp/supabase-installer-task8-round1-audit-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/project`: PASS.
- `git diff --check a1c1969..203ec9d`: FAIL at `apps/web/src/features/project/configuration/SecretsSection.tsx:17`.

Round 1 verdict: **NOT APPROVED**. No Critical finding was identified, but multiple Important failures remain and there is no new test evidence for the rewrite.

---

# Task 8 Fix Round 2 Review — NOT APPROVED

Round 2 scope: `203ec9d..092438b`. The 11 Round 1 Important findings and whitespace Minor were rechecked against the exact diff and current production paths. The report was not accepted as evidence; only one existing Go assertion changed, while no Web test changed.

## Critical

None.

## Important

- **Important — `apps/web/src/features/project/configuration/ConfigurationPage.tsx:39-48` plus every section reset effect (for example `AuthSection.tsx:9-10`): Round 2 destroys dirty form state whenever the parent rerenders.** `normalizeRedactedConfiguration()` constructs fresh Auth/Database/Functions objects on every render, and each section treats the new `initial` reference as server data and calls `form.reset(initial)`. Submitting once calls `setPending`, which rerenders the parent and clears the form before the preview is acted on; “Keep editing” therefore loses the edit. A 409 calls `setConflict` and resets the form the same way, directly falsifying “Your dirty fields are preserved.” This is a new load-bearing regression introduced by the omitempty normalization.

- **Important — `apps/web/src/features/project/configuration/types.ts:30-38`, `AuthSection.tsx:9-18`, and `OAuthSection.tsx:12-18`: redacted normalization remains incomplete, so valid real responses still cannot pass section schemas.** It defaults redirect URLs, the OAuth map, extensions, variables, and Phone fields, but not omitted `auth.phone.provider` nor omitted `fields` on each OAuth provider. Default Go JSON omits Phone provider, while `phoneUpdateSchema` requires a string; a configured Google provider normally omits fields, while `oauthProviderSchema` requires a record. The prior immediate `.join`/index crashes are closed, but normal Auth/OAuth saves can still be blocked by values the server legitimately omitted.

- **Important — `apps/web/src/features/project/configuration/schema.ts:10-33`, `useConfigurationMutation.ts:30-42`, and `ConfigurationPage.tsx:36`: section validation and authoritative field-error handling are unchanged.** General Domain, enabled SMTP, Phone, OAuth special fields, Storage endpoint/actions, and Functions environment/secret constraints still do not mirror Manager. `APIError.fields` is still reduced to a toast and cannot attach to the owning form. Round 2 made no schema or mutation-error change, so this Round 1 finding remains fully open.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:8-14`: unlocking the switches did not complete dependency-aware mutation.** Disabling Gateway closes public dependents, but re-enabling Storage/imgproxy, Studio, Auth, REST, Realtime, or Functions while Gateway/REST is off does not restore its required closure. Disabling REST while Storage remains on is likewise allowed locally. The schema then blocks submit, and its Toggle errors are not displayed. All non-database switches are visible now, but several valid OFF→ON paths remain unusable without manually discovering and toggling dependencies in the right order.

- **Important — `apps/web/src/features/project/configuration/types.ts:45-57` and `ConfigurationPage.tsx:40,48,51-57`: update preview remains wrong, and the new start/stop values are rendered as “No runtime restart expected.”** `sectionImpact` still labels SMTP/OAuth/Auth/Storage/Realtime/Functions as restart rather than recreate, and General still omits Studio despite the report claiming it was added. `serviceImpact` can now return `start` or `stop`, but the dialog only has branches for `recreate` and `restart`; both new impacts fall into the “No runtime restart expected” fallback. This is more misleading than Round 1 for the exact service transitions the patch claims to fix.

- **Important — `apps/web/src/features/project/configuration/GeneralSection.tsx:13-15`: the conflicting callback authority is unchanged.** General still derives and copies OAuth callback from Site URL, while provider cards correctly derive it from Domain/Project URL. Round 2 did not touch this section.

- **Important — `apps/web/src/features/project/configuration/OAuthSection.tsx:12-18` and `fields.tsx:20-34`: the shadcn/accessibility finding remains, with duplicate IDs across the 20 simultaneously rendered provider forms.** Raw inputs/selects/buttons/textareas remain throughout the rewrite. Every provider Client ID uses `id="field-clientId"` and every callback uses `id="readonly-callback-url"`, so labels do not uniquely identify their control when multiple providers are displayed. Toggle/Select errors, server field errors, and described error relationships remain absent. The always-visible OAuth footer correctly fixes provider disable, but it does not fix the required shadcn/a11y composition.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:7-15` and `apps/manager/internal/httpapi/configuration_test.go:33-65`: Round 2 still has no Web/API behavioral coverage for its corrections or regressions.** No Web test changed, so the suite still does not exercise real omitempty JSON, dirty preservation/Keep editing, 409, OAuth disable, dependency closure, service start/stop preview, four-kind reveal/no cache, navigation, schemas, field errors, or accessibility. The one-line Go assertion at `configuration_service_test.go:50` does cover Functions row deletion through the service, but there is still no HTTP test for the GET secret boundary or service-section synchronization. Passing 46 unchanged Web tests does not validate the Round 2 claims.

## Minor

None. `git diff --check 203ec9d..092438b` genuinely passes, so the prior whitespace/report mismatch is closed.

## Round 2 disposition of Round 1 findings

- **Closed:** OAuth providers now keep an always-visible Save action and can be disabled; configuration GET no longer decrypts or caches Anon Key and all four reveal kinds use the recent-auth/no-store endpoint; Functions remove now deletes the encrypted secret and row in one service mutation with a focused existing-test assertion; Logs, Backups, Project Settings, and Configuration have distinct routes/navigation; the immediate nil `redirectUrls`/OAuth-map/extensions crashes are normalized; all non-database service switches are visible.
- **Partially closed:** real redacted DTO handling still misses Phone provider and per-provider fields; Services is unlocked but closure is incomplete; navigation is fixed but has no regression test.
- **Still Important or regressed:** dirty/409 preservation, exact schemas and field errors, dependency-aware service changes, truthful preview, General callback authority, shadcn/accessibility, and Web/API behavioral tests.
- **Unchanged and sound:** OAuth per-provider endpoint, Storage Local remove path, rotation operation handoff, normal query invalidation, and the carried SMTP/OAuth sibling-action boundary.

## Round 2 verification

- `npm test --workspace apps/web -- --run`: PASS, 11 files / 46 tests (no Web test changes).
- `npm run build --workspace apps/web`: PASS; 818.83 kB chunk warning.
- `GOCACHE=/tmp/supabase-installer-task8-round2-audit-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/project`: PASS.
- `git diff --check 203ec9d..092438b`: PASS.

Round 2 verdict: **NOT APPROVED**. No Critical finding was identified, but eight Important findings remain, including a new dirty-state reset regression.

---

# Task 8 Fix Round 3 Review — NOT APPROVED

Round 3 scope: `092438b..8201700`. The eight Round 2 Important findings were rechecked against the exact production diff, the Manager validation and PATCH boundary, Provisioner affected-service calculation, and the newly added tests. Test names and the implementation report were not accepted as evidence by themselves.

## Critical

None.

## Important

- **Important — `apps/web/src/features/project/configuration/schema.ts:12-38`, `ConfigurationPage.tsx:43,51`, and `useConfigurationMutation.ts:41-55`: the UI schemas and authoritative field-error path are still not at Manager parity.** The Storage schema still accepts a non-URL object-store endpoint and accepts `remove` for a configured key on an object backend; the SMTP, OAuth, and Phone refinements likewise allow a configured secret's `remove` action while the corresponding feature remains enabled, although Manager rejects each case. The Network schema is inherited from the creation form and permits only `external|caddy`, while the Go contract/Manager also support `manual`; the Network UI exposes neither Manual Certificate nor its typed certificate field, so a legitimate existing manual configuration cannot be edited. Finally, only General supplies `pending.setError`; Auth, SMTP, OAuth, Storage, Functions, Services, and the other section callbacks drop it, leaving their `APIError.fields` only in the page-level summary rather than attached to the owning RHF field. Round 3 improves several validations, but does not close the exact-schema/field-local acceptance requirement.

- **Important — `apps/web/src/features/project/configuration/SMTPSection.tsx:10-11` and `fields.tsx:54-60`: explicit SMTP remove semantics remain unreachable for a normal configured-disabled state.** Disabling configured SMTP with the password unchanged retains the secret. On the subsequent redacted load `passwordSet` is still true, but the entire credential grid and `SecretEditor` are rendered only while `smtp.enabled` is true, so the administrator cannot select Remove in the state where Manager expressly permits removal. Selecting Remove before disabling happens to work, but an already-disabled configured password has no remove control. Phone Auth has the same hidden-editor behavior. This does not meet the required retain/replace/remove control contract despite the generic editor containing all three actions.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:12-13` and `apps/manager/internal/project/configuration_service.go:78-85,123-166`: dependency editing is still incomplete, and the new durable-boundary normalization silently changes API intent.** Enabling imgproxy sets only Storage, not Storage's REST/Gateway closure; disabling REST clears Storage but not imgproxy. The first path can either hit an undisplayed Toggle/schema error or, when Gateway is already on and REST is off, submit a preview that omits REST and let Manager silently enable REST. The second path is blocked by an undisplayed imgproxy error until the user discovers the dependency manually. At the backend, `normalizeServiceDependencies` runs before validation and prioritizes still-enabled children: for example `{gateway:false, auth:true}` is accepted and rewritten to `gateway:true`, and `{postgresMeta:false, studio:true}` is rewritten to `postgresMeta:true`. The approved domain rule says a direct invalid API payload is rejected; silently enabling services is neither rejection nor a deterministic parent-disable operation. The added Go “closes closure” test manually sets every dependent false before its close assertion, so it never exercises this ambiguity.

- **Important — `apps/web/src/features/project/configuration/types.ts:47-59` and `ConfigurationPage.tsx:43,51,53-59`: the preview labels are corrected but the preview is still not calculated from actual runtime impact.** Every Storage, Realtime, Functions, Auth, SMTP, and OAuth section update is labelled `recreate` with a fixed affected-service name even when that service is disabled. Provisioner intersects affected services with the newly enabled runtime, so editing Functions while `services.functions=false` or Storage while `services.storage=false` recreates nothing, while the dialog says a runtime recreate is required. For Services, affected names come from RHF dirty flags rather than the Manager-normalized result, which compounds the omitted dependency problem above. Start/stop badges, General's Studio entry, and recreate labels are fixed, but the required truthful affected/restart/recreate preview is only partially closed.

- **Important — `apps/web/src/features/project/configuration/AuthSection.tsx:13-20`, `OAuthSection.tsx:14-16`, `ServicesSection.tsx:12-13`, and `fields.tsx:27-60`: shadcn adoption and unique control IDs improved, but field errors and accessible relationships remain incomplete.** Text and number inputs now use generated Input/Field composition, unique `useId` IDs, `aria-invalid`, and `aria-describedby`. However Redirect URLs, Selects, Switches, and every SecretEditor have no access to their RHF error and render no `FieldError`/described error relationship; invalid service closure or secret actions can therefore make Save appear to do nothing. In the 20-provider OAuth grid, every configured secret also exposes indistinguishable “Retain” and “Remove” buttons without a provider-specific accessible name. This leaves the explicit labels, descriptions, field errors, and component-level accessibility acceptance open even though the raw-control and duplicate-ID defects are closed.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:35-102`, `configuration/types.test.ts:30-47`, `apps/manager/internal/httpapi/configuration_test.go:15-58`, and `apps/manager/internal/project/configuration_service_test.go:16-28`: the new tests are real executions but still do not substantiate the load-bearing Task 8 acceptance paths.** The dirty/Keep Editing and 409/Reload tests genuinely reproduce the prior reset regression, and the OAuth test genuinely sends the provider subresource. In contrast, the “authoritative field errors” test asserts only the global summary, supplies an already stripped `domain` path, and never checks `setError`, inline text, or `aria-describedby`; the normalization test calls no section schema despite calling its values schema-valid; the GET redaction test stores no secret plaintext before checking that none was returned; and the backend close test zeros all children itself. There remains no Web behavior test for SMTP/S3/Functions retain-replace-remove payloads and badges, conditional schema errors, service dependency UI/preview, four reveal kinds/no-cache/timeout/section clearing, rotation Operation flow, operation-terminal refetch, or accessibility. The additions are useful focused regressions, but the mandatory Task 8 matrix is still materially uncovered and fails to expose the production defects above.

## Minor

None. `git diff --check 092438b..8201700` passes.

## Round 3 disposition of the eight Round 2 findings

- **Closed:** normalized section values are memoized and forms reset only on a genuine server revision/remount, with real Keep Editing and 409/Reload coverage; omitted Phone provider/fields and OAuth provider fields are normalized; General and OAuth now use the same Domain-derived callback; preview renders start/stop and uses recreate labels; generated control IDs are unique and raw controls were largely replaced by shadcn primitives.
- **Partially closed / still Important:** schema parity and local Manager errors; deterministic Services closure; runtime-aware preview; shadcn/accessibility error relationships; and behavioral coverage.
- **Newly confirmed while checking those items:** configured-disabled SMTP/Phone secrets lack a reachable Remove action, and Manager's new dependency normalizer silently rewrites direct invalid service payloads instead of rejecting them.
- **Unchanged and sound:** OAuth provider endpoint and disable flow, normal GET secret boundary, Functions one-operation row/secret deletion, rotation Operation handoff, query invalidation after terminal success, navigation, and the carried SMTP/OAuth sibling secret-action fix.

## Round 3 verification

- `npm test --workspace apps/web -- --run`: PASS, 12 files / 53 tests.
- `npm run build --workspace apps/web`: PASS; 823.49 kB chunk warning.
- `GOCACHE=/tmp/supabase-installer-task8-round3-audit-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/project`: PASS.
- `git diff --check 092438b..8201700`: PASS.

Round 3 verdict: **NOT APPROVED**. No Critical finding was identified, but six Important findings remain.

---

# Task 8 Fix Round 4 Review — NOT APPROVED

Round 4 scope: `8201700..08478b2`. All six Round 3 Important findings were rechecked against the exact diff. The removal of Manual HTTPS was traced through Go contracts, Manager validation, Provisioner rendering/reconciliation, the Web DTO/schema/default payload, and strict HTTP decoding. Backend dependency rejection, preview semantics, disabled-secret controls, accessibility, tests, package-manager ownership, and the Auth section were checked independently of the report.

## Critical

- **Critical — `internal/contracts/configuration.go:164-180`, `apps/web/src/api/types.ts:48,58`, `apps/web/src/features/projects/projectSchema.ts:40,47-61`, `NewProjectPage.tsx:28`, and `apps/manager/internal/httpapi/auth_handlers.go:140-153`: deleting `NetworkConfig.Certificate` breaks every current Web project-create request at strict JSON decode.** The Web contract still declares `httpsMode: 'manual'` and `certificate`, and `defaultConfiguration()` puts `certificate: ''` into every Lightweight/Standard/Full/Custom aggregate. `normalizeCreateConfiguration()` recursively preserves that property, so `NewProjectPage` sends it to `POST /api/projects`. Manager decodes `ProjectDraft` with `DisallowUnknownFields`, but the Go `NetworkConfig` no longer has `certificate`; the request therefore returns `400 INVALID_JSON` before project validation or operation creation, even when HTTPS mode is the default `external`. The Web tests mock a 202 response and never cross the real decoder, so all 55 tests pass while the primary create workflow is broken. Independently, removing `HTTPSModeManual` and `Certificate` contradicts the approved typed contract/design rather than implementing its Manual Certificate flow, and leaves Go and TypeScript wire models inconsistent.

## Important

- **Important — `apps/web/src/features/project/configuration/ConfigurationPage.tsx:43,50-51`, `AuthSection.tsx:13-15`, `OAuthSection.tsx:14-16`, and `useConfigurationMutation.ts:41-55`: none of the newly added section-level Manager errors actually reaches `PendingConfigurationSave.setError`.** Non-OAuth sections call `onSave(value, dirty, setError)` with the callback in argument three, but the page's `save(value, dirty, provider, setError)` interprets argument three as an OAuth provider and leaves `setError` undefined. OAuth passes four arguments from `ProviderForm`, but the page wrapper accepts only `(provider, value, dirty)` and drops the fourth. The mutation hook therefore cannot call any section form's `form.setError`; only the global summary remains. Existing casts hide the signature mismatch from TypeScript, and the test still asserts only that global summary. Field-local authoritative errors remain non-functional.

- **Important — `apps/web/src/features/project/configuration/schema.ts:11,43-44`, `FunctionsSection.tsx:9-13`, and `apps/manager/internal/project/validate.go:52-71`: two material schema-parity failures remain.** Installed-project Domain rejects IP literals and host-with-port values that Manager and the creation schema accept, so a legitimately created local/IP project cannot save General without replacing its valid Domain. Functions rejects every redacted configured variable whose normal initial value is `valueSet:true` plus `{action:""}`; this happens before `normalizeConfigurationValue()` can convert unchanged configured leaves to `retain`. Consequently changing JWT verification or another variable while configured variables exist requires manually pressing Retain on every row, contrary to the masked-unchanged retain contract. Its SecretEditor also receives no error prop, so this blocking action error is not rendered at the field.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:13-35`: the common imgproxy/Storage/REST closure is repaired, but disabling Gateway still stops services that do not depend on Gateway.** The handler forcibly disables Supavisor, Logs/Vector, and the direct PostgreSQL port together with public Gateway dependents. Manager's authoritative rules require those services only to retain PostgreSQL (and Logs/Vector each other), not Gateway. An administrator turning off the public gateway can therefore unintentionally stop three independent private/observability capabilities; the backend correctly accepts the over-broad but valid resulting snapshot because the UI represents them as explicit changes. Backend silent normalization is closed and its new service-level rejection test is real, but the UI closure is still not the authoritative dependency graph.

- **Important — `apps/web/src/features/project/configuration/types.ts:48-73` and `ConfigurationPage.tsx:43,53-59`: preview logic still confuses a disabled feature's configuration with the act of disabling it.** `sectionImpact` returns `none` whenever an SMTP or OAuth payload has `enabled:false`. That is correct for removing an already-disabled stored secret, but wrong when the user changes enabled SMTP/OAuth to disabled: Provisioner detects the Auth configuration change and recreates the still-enabled Auth service, while the dialog says no runtime action. General can likewise list no affected services after filtering disabled Gateway/Auth/Studio yet still say recreate, and disabled Supavisor/Network-owner cases remain inconsistent. The new SMTP test covers only the already-disabled secret-removal case and cannot distinguish this transition.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:34-35`, `AuthSection.tsx:19-29,35-39`, `NetworkSection.tsx:9-11`, `RealtimeSection.tsx:8-12`, and `fields.tsx:27-31`: accessibility/error rendering is improved but remains incomplete at the exact controls that can block submission.** Toggle now supports an accessible error and SecretEditor has provider-specific action names and described errors, yet no Services/Auth Toggle supplies its RHF error. Auth's Phone provider Select, Network selects, Realtime log-level Select, and Storage backend Select likewise render no local schema/server error. Examples include disabling signup while anonymous/Phone/OAuth signup remains enabled, or disabling Gateway while Caddy remains selected: RHF or Manager rejects the save, but the responsible control exposes no error relationship. Combined with the broken server `setError` wiring above, the explicit field-error/accessibility acceptance is still open.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:71-84,104-118`, `configuration/types.test.ts:50-60`, `apps/web/src/features/projects/NewProjectPage.test.tsx:7-30,44-66`, and `apps/manager/internal/project/configuration_service_test.go:16-42`: Round 4 tests add useful real unit/UI coverage but still miss the integration boundaries that fail.** The SMTP disabled-removal test genuinely checks the remove payload and no-runtime preview; schema tests genuinely reject the listed secret/endpoint cases; and the Manager service test genuinely rejects an invalid Gateway closure without rewriting it. However the field-error test still checks only the global alert, preview has no enabled→disabled SMTP/OAuth case, service UI has no Gateway-independent-service case, and project creation is tested only against a fetch mock that unconditionally returns 202, so the Critical strict-decoder failure is invisible. Four-kind reveal/no-cache/timeout/section clearing, rotation Operation flow, operation-terminal refetch, and representative field-level accessibility also remain uncovered.

## Minor

None. `git diff --check 8201700..08478b2` passes.

## Round 4 disposition of the six Round 3 findings

- **Closed:** Manager no longer silently normalizes invalid service PATCHes and a real service test proves rejection; imgproxy enable and REST disable now close their Storage dependencies; configured-disabled SMTP and Phone reveal their remove controls; enabled configured-secret remove and Storage endpoint URL validation now match Manager; SecretEditor action names/IDs/error descriptions are improved.
- **Partially closed / still Important:** exact schemas and field-local errors; dependency-aware Services UI; runtime-aware preview; accessibility error relationships; and the acceptance test matrix.
- **Regressed to Critical:** removing Manual HTTPS/Certificate from Go without updating the Web wire/default/create payload makes every browser project creation fail strict decoding. It also deletes an approved contract capability rather than implementing it.
- **Package/Auth audit:** only npm is present (`package-lock.json`, `packageManager: npm@11.17.0`); no pnpm/yarn/bun lock or workspace exists. The installed-project Auth section, route selection, Manager Auth configuration endpoint, and authentication service remain present; no second Auth/configuration implementation was introduced in this diff.
- **Unchanged and sound:** dirty/409 preservation, redacted DTO normalization, callback authority, OAuth provider endpoint/disable, normal GET secret boundary, Functions backend row/secret deletion, rotation operation handoff, terminal query invalidation, navigation, and the carried SMTP/OAuth sibling secret-action fix.

## Round 4 verification

- `npm test --workspace apps/web -- --run`: PASS, 12 files / 55 tests.
- `npm run build --workspace apps/web`: PASS; 826.15 kB chunk warning.
- `GOCACHE=/tmp/supabase-installer-task8-round4-audit-go-cache go test ./apps/manager/internal/httpapi ./apps/manager/internal/project ./apps/provisioner/internal/render ./apps/provisioner/internal/runtime ./internal/contracts`: PASS.
- `git diff --check 8201700..08478b2`: PASS.

Round 4 verdict: **NOT APPROVED**. One Critical and six Important findings remain.

---

# Task 8 Fix Round 5/5 Final Review — NOT APPROVED

Round 5 scope: `08478b2..ede1668`. The Round 4 Critical and six Important findings were rechecked against the exact production diff and current Go/Web contracts. At the review cap, findings below distinguish release-load-bearing defects from gaps that can be parked without claiming their missing coverage exists.

## Critical

None. The prior strict-create Critical is closed: `apps/web/src/api/types.ts:48-58` and `apps/web/src/features/projects/projectSchema.ts:40,47-61` now remove `manual`/`certificate` from the Web wire schema, defaults, redacted DTO, and normalized create aggregate, matching the strict Go `NetworkConfig`. The browser no longer sends the deleted `certificate` member to `POST /api/projects`.

## Important — load-bearing

- **Important — `apps/web/src/features/project/configuration/ConfigurationPage.tsx:43,50-51`, `AuthSection.tsx:13-15`, `OAuthSection.tsx:14-16`, and `useConfigurationMutation.ts:41-48`: Round 5 still does not deliver authoritative Manager errors to any section form.** Non-OAuth sections still call `onSave(value, dirty, setError)`, while the page still interprets argument three as `provider`, so `PendingConfigurationSave.setError` is undefined. OAuth still passes four arguments from its provider form into a page wrapper accepting only three and drops its callback. The casts at the page hide the mismatch; importing `APIError` in the mutation hook does not repair the missing callback. The added `GeneralSection.serverErrors` effect is also unused because the page never passes that prop. A 422 therefore produces only the page summary, not RHF field errors or control-local accessible feedback.

- **Important — `apps/web/src/features/project/configuration/ServicesSection.tsx:13-32` and `apps/manager/internal/project/configuration.go:120-157`: Gateway dependency closure is still over-broad.** Turning Gateway off no longer disables Supavisor, Logs/Vector, or Direct DB, but line 29 still forces `postgresMeta:false`. Manager requires Gateway only for Auth, REST, Studio, Realtime, Storage, and Functions; postgres-meta is independent once Studio is disabled. The UI therefore still turns off a supported service the administrator did not request to stop. The new page test calls itself “only public dependents” but never asserts that the postgres-meta switch stays checked, so it misses the remaining error.

- **Important — `apps/web/src/features/project/configuration/types.ts:48-73` and `ConfigurationPage.tsx:43,51-58`: preview old/new handling is repaired only for SMTP/OAuth enablement transitions; disabled runtime owners remain contradictory.** The new `previous` argument correctly distinguishes enabled→disabled from secret removal on an already-disabled SMTP/OAuth configuration. However General or Network changes with Gateway/Auth/Studio disabled, and Pooler changes with Supavisor disabled, still filter the affected-service list to empty while unconditionally reporting “Runtime recreate required.” The dialog can simultaneously say “Configuration metadata only” and “Runtime recreate required,” so affected/recreate behavior is not yet derived consistently from the actual enabled runtime.

- **Important — `apps/web/src/features/project/configuration/schema.ts:16-38` and `AuthSection.tsx:19-39`: client-side blocking Auth errors remain invisible and inaccessible.** Enabling Phone Auth without a provider creates a `phone.provider` Zod error, but the provider Select has no rendered error, `aria-invalid`, or `aria-describedby`. Disabling signup while Phone/anonymous/OAuth signup is enabled creates a `disableSignup` error on a hidden mirrored field; none of the visible controls renders it. `handleSubmit` consequently blocks before the page-level API summary exists, and Save appears to do nothing. Round 5 adds accessible error relationships to several Toggles and Selects, but it does not close the exact controls that still block submission; the broken server-error callback above compounds this.

- **Important — `apps/web/src/features/project/ConfigurationPage.test.tsx:71-135`, `configuration/types.test.ts:61-73`, `apps/web/src/features/projects/projectSchema.test.ts:87-92`, and `apps/manager/internal/httpapi/configuration_test.go:73-93`: the final acceptance suite still does not exercise the boundaries that remain broken.** The new tests genuinely cover configured Functions normalization, SMTP old/new impact, and preservation of Supavisor/Logs/Direct DB. But the field-error test still asserts only the global summary; the Gateway test omits postgres-meta; the strict-create check is a Web structural unit plus an unrelated Network PATCH rejection rather than a real strict `POST /api/projects`; and there is still no direct acceptance for the four reveal kinds/no-store/timeout/section clearing, rotation-to-OperationPanel flow, or representative field-level accessibility. The 59 passing Web tests therefore remain a false-negative for the first four findings.

## Parkable at the review cap

- Manual TLS/certificate remains absent rather than implemented as the original design described. That is now a consistent Go/Web capability decision and avoids exposing the pinned renderer's known failing path, matching the Task 7 final ruling; it no longer breaks ordinary creation and is not treated as a Round 5 release blocker.
- Four-kind reveal and database rotation production flows were unchanged this round and remain logically sound on re-audit: all four kinds use password re-entry and the reveal endpoint, plaintext stays in section component memory with timeout/unmount clearing and copy/hide controls, Manager sends `Cache-Control: no-store`, rotation shows the warning and hands the returned operation to `OperationPanel`, and terminal success invalidates configuration/project queries. Their missing direct acceptance coverage is debt; by itself it would be parkable, although the overall acceptance finding above remains load-bearing because the suite also misses current production defects.
- The Vite 827.02 kB chunk warning is performance debt, not a Task 8 correctness blocker.

## Round 5 disposition of the Round 4 findings

- **Closed:** strict create DTO mismatch; installed Domain parity for IP/host:port; configured Functions empty-marker normalization to `retain`; SMTP/OAuth enabled→disabled preview distinction.
- **Partially closed / still Important:** Gateway closure (postgres-meta remains over-closed); field-local Manager errors; control-level accessibility; runtime-aware preview; acceptance coverage.
- **Unchanged and sound:** backend rejection of invalid service closure and empty actions on configured target secrets; carried SMTP/OAuth sibling-retain fix; 409 dirty preservation; single configuration workspace/query-key/refetch flow; four reveal kinds and rotation operation implementation.

## Verification

- `git diff --check 08478b2..ede1668`: PASS.
- `npm test --workspace apps/web -- --run`: PASS, 12 files / 59 tests.
- `npm run build --workspace apps/web`: PASS; existing 827.02 kB chunk warning.
- `GOCACHE=<fresh temp dir> go test ./apps/manager/internal/httpapi ./apps/manager/internal/project ./apps/provisioner/internal/render ./apps/provisioner/internal/runtime ./internal/contracts`: PASS.

Final Round 5/5 verdict: **NOT APPROVED**. The prior Critical is resolved, but four production Important defects plus the acceptance false-negative remain load-bearing; the cap does not convert them into approval.
