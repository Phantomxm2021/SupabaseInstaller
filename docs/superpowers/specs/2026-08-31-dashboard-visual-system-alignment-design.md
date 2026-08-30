# Dashboard Visual System Alignment

## Goal

Align every Manager interface with the current Supabase Dashboard visual system while retaining all Manager-specific copy, routes, data, permissions, and workflows. “Alignment” includes layout geometry, typography, color, borders, radii, elevation, control dimensions, states, responsive behavior, and empty/loading/error states; it is not limited to similar-looking page sections.

## Product Boundaries

- Keep Manager terminology and content such as “Server”, “Manager-owned”, local branch labels, deployment operations, and self-hosted configuration.
- Do not add Supabase Cloud-only behavior, billing, organization management, or external API dependencies.
- Keep current APIs, mutation payloads, route paths, keyboard behavior, accessibility labels, and operation handling unchanged unless a visual component requires a non-breaking markup adjustment.
- Treat the Supabase Dashboard currently open in Chrome as the primary visual reference. Extract computed values from it instead of estimating them.

## Reference Contract

The baseline captured from the official Edge Function Secrets page establishes the shared compact-control language:

| Element | Reference value |
| --- | --- |
| Page title | Manrope, 22px, weight 600, 29.3333px line height |
| Heading label / table metadata | Source Code Pro, 12px, weight 600, 16px line height |
| Form input | Source Code Pro, 13px, weight 450, 34px control height |
| Form textarea | Source Code Pro, 13px, weight 450, 80px default height, 6px radius |
| Compact button | Inter, 12px, weight 450, 34px height, 6px radius |
| Card and input radius | 6px for compact controls, 7–8px for grouped surfaces |
| Theme | Near-black surfaces, low-opacity cool-green/gray borders, off-white foreground, emerald primary action |

The full audit must re-check these metrics in the current Dashboard before migration. The implementation must use CSS custom properties and component variants rather than copying raw values into individual page styles.

## Architecture

### 1. Design tokens and layout foundation

Replace the conflicting light Shadcn defaults and scattered hard-coded values in `styles.css` with a single dark Dashboard token layer. It defines background/surface tiers, foreground hierarchy, border opacity, state colors, spacing scale, radii, typography families, desktop shell dimensions, and responsive breakpoints.

The top bar, global icon rail, project workspace sidebar, and page content shell become layout primitives. Their dimensions and fixed/sticky behavior are shared rather than recreated by Functions, Authentication, Configuration, or project pages.

### 2. Primitive component contract

Normalize `Button`, `Input`, `Textarea`, `Select`, `Checkbox`, `Switch`, `Badge`, `Card`, `Table`, `Dialog`, `Sheet`, `DropdownMenu`, `Tooltip`, `Progress`, `Skeleton`, and `Empty` to consume the token layer. Each primitive exposes only needed Dashboard variants: primary, secondary/outline, ghost, destructive, compact, icon, and table/action variants. Hover, focus-visible, disabled, loading, selected, error, and destructive states are defined centrally.

No route page may override primitive typography, height, border radius, or colors except through an intentional named variant. Existing page selectors that target broad implementation details such as `[data-slot="button"]` are replaced with component variants or narrowly scoped layout rules.

### 3. Shared page patterns

Introduce reusable application patterns for page headers, section headers, compact filter/tool bars, settings rows, table shells, empty states, confirmation dialogs, operation status, and form action footers. These preserve each page’s existing functionality but eliminate repeated bespoke CSS.

### 4. Route migration groups

Migrate in visual dependency order:

1. Shell and primitives: authenticated top bar, global rail, mobile nav, dialogs, menus, tables, forms, feedback.
2. Entry and project navigation: Login, Setup, Projects, New Project wizard, Project Overview, Manager Settings.
3. Configuration: Server Settings navigation plus every General, Services, Auth, Storage, Realtime, Functions, Database, Pooler, and Network section.
4. Authentication: workspace nav, Users, OAuth Apps, Sign-in Providers, Emails, template editor, rate limits, multi-factor, URL configuration, unavailable states.
5. Functions: workspace nav, Deployments dialog/status, Managed functions table, Secrets.
6. Cross-cutting states: empty, loading, error, pending/operation, disabled service, confirmations, narrow/mobile layouts.

## Responsive Rules

- Desktop (>= 1280px): fixed 48px top bar, 48px global rail, fixed 256px workspace sidebar where applicable, content widths matching the Dashboard reference.
- Medium (768–1279px): preserve information hierarchy; tables horizontally scroll inside their own shells; avoid global horizontal overflow.
- Mobile (< 768px): global and workspace navigation move to Sheets; page padding, toolbars, headers, multi-row forms, and dialog footers stack intentionally; controls remain touch-safe without inflating desktop density.
- Test the three widths 1440px, 1024px, and 375px for every migrated route family.

## Verification

- Add unit/component tests for primitive variants and the observable route behaviors affected by markup changes.
- Keep the full existing frontend and Go suites passing.
- Build production assets after each migration group.
- Compare each migrated route to the Dashboard reference in Chrome at the three viewport widths. Review title metrics, shell offsets, control dimensions, text sizes/weights/colors, border/radius, state appearance, and scroll behavior before moving to the next group.
- Do not mark a page family complete while it relies on an ad-hoc CSS override for a primitive that should be shared.

## Completion Criteria

The work is complete only when every implemented route listed in `app/router.tsx`, every public UI primitive, and every state variant above follows the shared Dashboard design tokens; Manager-specific copy and behavior remain intact; and desktop/tablet/mobile visual checks plus automated tests and production builds are green.
