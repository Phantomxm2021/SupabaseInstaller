# Runtime settings discoverability design

## Goal

Make the collapsed runtime control in the create-server wizard visibly
interactive and explain what it controls without expanding it.

## Interaction

`Runtime settings` becomes `Advanced runtime settings`. The complete header is
one button with a clear chevron affordance and an explicit action label:
`Expand settings` while closed and `Hide settings` while open. The header also
contains the safe, non-secret summary `Supabase self-hosted/v0.8.0 · 1 setting`.

Opening it reveals the existing pinned Supabase-version selector. The full
header remains keyboard-operable and exposes the Collapsible primitive's
`aria-expanded` state. No configuration data, defaults, or validation behavior
changes.

## Visual and content rules

- Use the current Card, border, muted-text, and Lucide icon language; do not
  introduce a new component family.
- The title identifies the control category; its description identifies its
  only current setting and that the version is pinned.
- Chevron rotation and action text make state discoverable without hover.
- The summary stays visible in both states so operators can confirm the chosen
  runtime while the detail is open.

## Verification

Extend the NewProjectPage test to assert the closed-state title, description,
summary, explicit expand affordance, `aria-expanded` transition, and selector
visibility after activation. Run the focused test, the Web suite, typecheck and
production build.
