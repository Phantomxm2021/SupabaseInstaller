# Shadcn Experience Redesign

**Date:** 2026-08-29  
**Status:** Approved design — pending document review

## Goal

Improve the desktop management experience by making shadcn/ui the shared interaction and composition baseline. The result keeps the product's dark Supabase-inspired brand and green accent, but replaces inconsistent page-specific presentation with predictable components, states, and motion.

The first delivery wave covers:

1. Project list and project detail
2. Create-project wizard
3. Authentication
4. Project settings

Mobile layouts are outside first-wave acceptance. Pages must nevertheless remain usable without a broken layout at narrower desktop widths.

## Design Direction

Use the existing shadcn base-nova setup and Base UI primitives. Do not introduce a second component library or change the backend technology.

Adopt a branded shadcn direction:

- Preserve the dark surface palette and Supabase green as the primary action/status color.
- Use shadcn component structure, sizing, focus behavior, composition, and feedback patterns.
- Prefer dense but readable administration screens over decorative gradients, glass effects, or high-motion presentation.

## Component Architecture

Maintain two explicit UI layers.

### Primitive layer: components/ui

This layer contains shadcn component source and its immediate helpers only. It is the sole home for reusable primitives such as Button, Field, Input, Select, Combobox/Command, Dialog, Sheet, Alert Dialog, Table, Tabs, Sidebar, Skeleton, Spinner, Tooltip, and Sonner.

Existing components are kept on the shadcn Base UI route and aligned with the generated upstream implementation as they are touched. New primitives are added with the shadcn CLI rather than hand-built lookalikes.

### Application layer: components/app

This layer contains product-specific, reusable compositions:

- PageHeader
- FormSection
- DataState (loading, empty, error)
- StatusCard
- DataListToolbar
- ConfirmDestructiveAction

Feature components compose these application pieces and primitives; they do not define new global control variants.

## Styling System

Refactor the current large global stylesheet into:

1. Design tokens: colors, typography, radii, spacing, shadows, z-index, and motion durations.
2. Global base styles and accessibility defaults.
3. Small shared layout utilities.
4. Feature-local styles only where Tailwind composition is insufficient.

The source of truth for spacing, foreground/background, destructive state, and focus rings is the shadcn token set extended by the dark Supabase brand tokens. Avoid page-specific overrides of primitive button, field, dialog, and table states.

## Shared Interaction Rules

Every async interaction uses a consistent state model:

- Initial fetch: Skeleton where content shape is known.
- Empty collection: Empty state with contextual primary action.
- Recoverable query failure: Alert with retry action.
- Submission: disable only the relevant action and show Spinner.
- Completion: success or error Sonner notification.
- Destructive action: Alert Dialog confirmation before the mutation.

All controls expose visible keyboard focus and accessible labels. Dialogs, sheets, menus, and comboboxes must preserve expected keyboard navigation and focus return behavior.

Motion is limited to spatial feedback:

- wizard forward/back transitions;
- dialog and sheet open/close;
- collapsible content;
- row/state updates where appropriate.

Motion duration is 150–220 ms. The system reduced-motion preference removes directional movement and preserves only an effectively instant fade where needed.

## First-Wave Screen Designs

### Project List and Detail

Use a compact page header with a primary Create project action. The list supports search, clear status badges, loading skeletons, an explicit empty state, and per-row actions in a Dropdown Menu.

The project detail view uses the same header and status cards, then groups configuration and operational actions into predictable sections. Status is conveyed with Badge and text, not color alone.

Primary components: Data Table, Input Group, Badge, Dropdown Menu, Empty, Skeleton, Card, Sheet, Alert Dialog, and Sonner.

### Create-Project Wizard

The wizard remains desktop-first and single-column for basic project information. It preserves the agreed interaction rules:

- Admin username is always displayed and is never hidden in a collapsed section.
- Project name is checked for format and availability while editing; the server remains the final duplicate check on submission.
- The step frame animates forward and backward.
- Advanced configuration is designed and present in the review flow.
- Security and integrations omit redundant email-login-already-enabled summary rows.
- A switch belongs at the far right of its collapsible setting row.
- Authentication methods are added dynamically rather than pre-rendering every scheme.
- Choosing an OAuth provider enables it immediately; it has no enable switch.
- Enabled OAuth providers have an icon-only remove action, with an accessible label and confirmation where configuration loss warrants it.
- The add-authentication dialog has search and a single-column result list of all available methods.

Primary components: Field, Input, Alert, Button, Collapsible, Dialog, Command/Combobox, Switch, Alert Dialog, Spinner, and Sonner.

### Authentication

Enabled methods appear as a single-column list of status rows. Add authentication method opens a searchable Dialog with login methods and OAuth providers. A chosen OAuth provider is immediately added/enabled. Removal is an accessible icon button, not a toggle.

Primary components: Dialog, Command/Combobox, Button, Tooltip, Alert Dialog, Badge, Field, Input, and Sonner.

### Project Settings

Group general information, integrations, and destructive actions into distinct sections. Place integration switches at the far right of the controlling row. Use Sidebar or Tabs only where there is sufficient setting depth; do not create a first-line status summary that duplicates the active control.

The danger zone uses destructive Button styling and Alert Dialog confirmation.

Primary components: Sidebar or Tabs, Field, Input, Switch, Separator, Alert Dialog, Skeleton, and Sonner.

## Data and Backend Boundary

This redesign does not change Go backend behavior, install execution, persisted configuration semantics, or public API contracts.

The frontend continues to use existing query and mutation hooks. Project identity availability remains an asynchronous client check followed by server-side final validation on submit. Mutations invalidate/refetch the relevant existing queries and present their result through the shared async state model.

## Delivery Order

### Batch 1: foundation and wizard

1. Normalize shadcn primitive sources and add the missing approved primitives.
2. Introduce tokens and application-level shared compositions.
3. Refactor the create-project wizard and its authentication picker.
4. Add component and user-flow tests.

### Batch 2: management pages

1. Refactor project list and detail.
2. Refactor authentication and project settings.
3. Apply shared empty, loading, error, destructive-confirmation, and notification patterns.
4. Complete interaction and regression testing.

## Acceptance Criteria

- First-wave desktop pages use the shared shadcn primitive/application layers rather than local lookalike controls.
- The agreed wizard behavior is retained, including single-column basic information, visible admin username, project-name legality and duplicate checks, animated step changes, searchable authentication addition, immediate OAuth enablement, and icon-only OAuth removal.
- Async states, empty states, validation feedback, focus states, and destructive confirmations follow the shared interaction rules.
- Motion honors the system reduced-motion preference.
- Existing backend behavior and configuration semantics are unchanged.
- Existing business tests continue to pass; new tests cover keyboard/focus behavior, wizard validation, loading/error feedback, and OAuth add/remove flows.
- Each batch passes web lint, web build, frontend tests, repository tests, and Go tests before handoff.

## Out of Scope

- Mobile-first redesign or mobile-specific acceptance.
- A new visual brand direction, a new component library, or a backend/API redesign.
- Redesigning operations, system pages, and lower-frequency workflows in the first wave.
