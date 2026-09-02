# Task 4 report

- RED: `go test ./internal/templates ./apps/manager/internal/project -run 'TestSupavisorBootstrap|TestFunctionsConfiguration' -count=1` failed as expected: missing `update_tenant` and serialized `directory`.
- GREEN: targeted package suite and `go test ./... -count=1` pass.
- Supavisor API proof: upstream `https://github.com/supabase/supavisor/blob/v2.9.5/lib/supavisor/tenants.ex`, tag `v2.9.5` commit `9cf7560b2b2d6e2f72711c1e2a131dc0b2ea6463`; exact signature is `def update_tenant(%Tenant{} = tenant, attrs)`, returning `{:ok, %Tenant{}} | {:error, %Ecto.Changeset{}}`. Bootstrap passes the loaded tenant and `Map.take(params, ["default_max_clients", "default_pool_size", "users"])`.
- Pooler bootstrap now creates missing tenants and updates existing tenant capacity/users while retaining match-error behavior for failed writes.
- Removed `FunctionsConfig.Directory` from DTO/defaults and migration seed; migration 016 strips the historical key from canonical and historical JSON. Managed function release trees/volumes remain untouched.
- Files: contracts/configuration.go; manager project defaults/tests; migration 002 + 016; pooler.exs; store migration tests; runtime/integration test fixtures.
- Commit: `fix: apply persistent pooler configuration`.
