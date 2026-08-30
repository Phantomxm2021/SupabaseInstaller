# Functions deployment status and secondary navigation design

## Scope

Correct two parts of the Functions workspace: deployment feedback stays inside the deployment dialog, and Functions navigation becomes a sidebar secondary menu instead of in-page tabs.

## Deployment dialog

After the deployment endpoint accepts the ZIP, the dialog remains open and switches from its upload form to a dedicated operation-status view. The view reuses the existing operation title, status badge, current step, progress, and error details. It hides the archive/name fields and deploy controls while the operation is active or terminal. Closing the dialog does not cancel the server-side operation; the page-level operation card remains available after dismissal.

## Secondary navigation

The project sidebar's Functions row becomes a collapsible parent. Its nested links are Deployments and Secrets, with active state derived from the current route. The in-page `FunctionsNavigation` tabs are removed from both Functions pages. Existing Deployments and Secrets routes remain unchanged.

## Verification

Tests will prove that a queued deployment leaves the dialog open and renders status within it, and that Functions child links are rendered by the sidebar rather than as page tabs. Existing deployment, rollback, deletion, and secrets behavior remain covered by the full frontend suite.
