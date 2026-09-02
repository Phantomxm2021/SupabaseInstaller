# Task 1 Report: Typed policy and validation

## RED

Command:

```text
go test ./apps/manager/internal/project -run 'TestConfigurationRejects(PhoneMFAWithoutProvider|NewCaddyValue)' -count=1
```

Result: failed as expected. Phone MFA without Phone Auth returned nil, and a new Caddy configuration was accepted.

## GREEN

Commands and results:

```text
go test ./apps/manager/internal/project -run 'TestConfigurationRejects(PhoneMFAWithoutProvider|NewCaddyValue)' -count=1
ok

go test ./internal/contracts ./apps/manager/internal/project -count=1
ok
```

## Changes

- Added `StorageConfig.UploadFileSizeLimit int64` and a 50 MiB default.
- Added 1 MiB–5 GiB upload-limit validation (zero remains accepted for pre-field persisted configurations).
- Added strict lowercase 32-hex R2 Account ID validation and required R2 path style.
- Added Phone MFA dependency validation requiring enabled/provider-configured Phone Auth with retained or replacement secret.
- Rejected new Caddy values while allowing legacy Caddy through stored-configuration validation.
- Added focused regression tests for all of the above.

Commit: `9edbee7a8ed61cc7ecd61fcbccc4d1617c24705e` (`fix: validate storage and auth configuration dependencies`)

## Concerns

The upload limit value `0` is treated as an omitted legacy field to avoid rejecting older stored/request fixtures; new defaults always set 50 MiB. Renderer/UI work in later tasks must preserve that compatibility behavior when materializing the value.

## Terra review corrections

- `PreparePatch` now permits an unrelated patch to an inherited legacy Caddy configuration, while still rejecting Caddy when introduced or changed from another mode.
- Upload limit `0` is rejected for incoming section/full configuration patches. Zero remains accepted only by stored/create compatibility validation for pre-field data.
- Regression tests: `TestConfigurationServiceAllowsUnrelatedPatchOnLegacyCaddy` and `TestConfigurationServiceRejectsZeroUploadLimitPatch`.
- Verification: `go test ./apps/manager/internal/project -run 'TestConfigurationService(AllowsUnrelatedPatchOnLegacyCaddy|RejectsZeroUploadLimitPatch)' -count=1` (pass); `go test ./internal/contracts ./apps/manager/internal/project -count=1` (pass).
