# Legacy Configuration Section Patch Compatibility Design

## Problem

Existing projects can persist compatibility-era omissions such as `auth.jwtExpiry = 0`. Runtime Sync normalizes these omissions, but ordinary section PATCH operations merge against the raw stored aggregate and validate the whole project. As a result, adding any Function secret returns 422 for an unrelated legacy Auth field.

## Design

Normalize the stored merge base with `NormalizeStoredConfiguration` before applying a section patch. Apply the incoming section after normalization so an explicit invalid value submitted by the user remains invalid. This makes compatibility defaults consistent between Runtime Sync and ordinary updates without weakening current validation.

## Verification

Exercise the real HTTP Functions PATCH against a project stored with `JWTExpiry = 0`, including an encrypted replacement secret, and require HTTP 202. Retain existing validation tests that reject an explicitly submitted zero JWT expiry.
