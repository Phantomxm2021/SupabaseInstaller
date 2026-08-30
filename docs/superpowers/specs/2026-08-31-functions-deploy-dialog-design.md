# Functions deploy dialog design

## Scope

Redesign only the deployment entry point on the Functions Deployments page. The existing Managed functions card, release data, action menu, rollback flow, delete flow, operation tracking API, and deployment endpoint remain unchanged.

## User experience

The page header exposes a right-aligned `Deploy` button using the existing Button component. Selecting it opens a standard Dialog titled `Deploy a function`.

The dialog contains the existing function name and ZIP archive fields, ZIP-layout guidance, the selected-file name, and Cancel / Deploy function actions. The deploy action uses the existing upload mutation and keeps its existing validation, disabled states, progress indicator, queued-operation toast, and operation status card.

Selecting `Deploy new version` from a managed function's Actions menu opens the same dialog and pre-fills that function's name. The name remains editable. Closing the dialog does not submit a deployment and clears no values, allowing a user to reopen it without losing a selected archive.

When the Functions service is disabled, the page shows the existing service-enable guidance and the dialog's deploy action is disabled. While an operation is running, deploying is disabled from both entry points.

## Component boundaries

`FunctionsPage` continues to own the API mutations, file/name state, and operation tracking. It gains only a `deployDialogOpen` state and a small open helper for new deployments or a selected existing function. The dialog is composed solely from the existing Dialog, Button, Input, Label, Alert, and Progress primitives; no bespoke modal implementation is introduced.

The header uses the existing `PageHeader.actions` slot. Legacy upload-card markup and CSS selectors are removed. Styling for the Managed functions card is intentionally untouched.

## Verification

Add UI tests proving that the header Deploy button opens the dialog, and that Deploy new version opens the same dialog with the matching name. Keep the existing archive-deployment test and adapt it to submit from the dialog. Run the focused test, full web test suite, web production build, and the repository Go tests before committing.
