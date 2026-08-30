# Functions Secrets Submenu Design

## Goal

Keep Edge Function deployment and its runtime secrets together by giving the
Functions workspace a two-item secondary navigation: Deployments and Secrets.

## User experience

`/projects/:projectId/functions` remains the default Deployments view with the
existing ZIP upload, release history, rollback, and delete controls.

`/projects/:projectId/functions/secrets` shows the existing Functions
environment-variable editor. The two routes share a compact secondary
navigation at the top of the Functions workspace, with the current item marked
as active. Browser navigation, deep links, and refreshes retain the selected
view.

## Architecture

The new Secrets route is a lightweight Functions workspace page. It reuses the
existing encrypted configuration API, `FunctionsSection`, schema validation,
write-only secret editor, confirmation workflow, and operation handling from
the Server Settings Functions section. No new secret API, plaintext response
field, or duplicate secret persistence is introduced.

The configuration page continues to expose its existing Functions section, so
current links and workflows remain valid. The Functions workspace simply gives
that same capability a contextual route beside deployments.

## Security and errors

Secret values remain write-only: reads only expose each variable's configured
marker, while replacement values stay in component memory until submission.
The new page retains existing validation, explicit retain/remove/replace
semantics, mutation confirmation, operation progress, field errors, and
revision-conflict behavior.

## Testing

Add route coverage for `/functions/secrets`, a UI test that proves the
secondary navigation switches between Deployments and Secrets, and a Secrets
view test that verifies the configuration endpoint is used without rendering a
stored plaintext value. Existing Functions deployment and configuration tests
remain the regression suite.
