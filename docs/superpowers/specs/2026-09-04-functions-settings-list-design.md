# Functions Settings List and JWT Expiry Design

## Scope

Improve two existing configuration surfaces without changing API contracts: expose JWT expiry in the active Authentication workspace, and replace the Project Settings Functions environment-variable stack with a compact list editor.

## Authentication

Add a Session settings row to Sign In / Providers with a numeric `JWT expiry (seconds)` control. Use the server-supported range 1–604800. Saving continues to submit the complete Auth section through the existing confirmation and reconciliation flow.

## Functions environment variables

Render a semantic table with Name, Value, Status, and Actions columns. Existing names remain visible and write-only values show Configured. Typing a value marks Replace pending. New rows accept a name and value. Removing an unsaved row deletes it immediately; removing a configured row marks it Pending removal and offers Undo until Save. Plaintext remains confined to component/form memory and is never returned by reads.

## Verification

Component tests cover table semantics, adding a replacement, configured-variable removal, undo, and submitted secret actions. Authentication tests cover the JWT expiry field, supported bounds, and the Auth PATCH payload. Run the complete web suite, build, lint, and Go suite.
