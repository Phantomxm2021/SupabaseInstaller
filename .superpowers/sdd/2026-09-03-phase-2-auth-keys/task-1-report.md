# Task 1 report: generate a validated key bundle

## RED

Before adding `internal/authkeys/bundle.go`,
`go test ./internal/authkeys ./internal/contracts -count=1` failed to compile
with `undefined: Generate` from the new auth-key tests.

## GREEN

After implementation:

- `go test ./internal/authkeys ./internal/contracts -count=1` passes.
- `go test ./... -count=1` passes across all Go packages.

## Crypto behavior

- Generates a P-256 EC signing key from the supplied `io.Reader`.
- Emits ES256 JWTs using SHA-256 and fixed-width IEEE P-1363 `r || s`
  signatures, with `anon` and `service_role` claims and matching `kid`.
- Emits `JWT_KEYS` with the private EC `d` member and legacy HS256 `oct`;
  emits `JWT_JWKS` with public EC coordinates and legacy `oct`, never EC `d`.
- Emits checksummed `sb_publishable_` and `sb_secret_` opaque keys.
- Validation rejects wrong/weak legacy secrets, malformed or partial JWK JSON,
  mismatched private/public EC material, altered opaque checksums, JWT claim or
  signature tampering, and missing asymmetric role tokens.
- JWT headers and role claims use strict typed structs with exact field sets;
  duplicate/unknown fields, non-integer timestamps, and invalid `iat`/`exp`
  ordering are rejected before signature verification.
- The six new `ProjectSecrets` fields are intentionally `json:"-"` until
  Task 3 introduces the dedicated private Manager/Provisioner envelope; they
  remain available for in-memory rendering paths.

## Commit

`feat: generate asymmetric auth key bundles`
