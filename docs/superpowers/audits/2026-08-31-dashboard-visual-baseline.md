# Dashboard Visual Baseline Audit

Reference: logged-in Supabase Dashboard, Edge Functions → Secrets, captured in Chrome at 1440px.

| Surface | Official Dashboard | Current Manager before primitive migration | Required alignment |
| --- | --- | --- | --- |
| Page title | Manrope 22px / 600 / 29.3333px, off-white | Manrope 16px / 600 / 24.8889px in the Functions workspace | Shared page-title variant |
| Section label | Source Code Pro 12px / 600 / 16px | ui-monospace 13px / 450 / 18.5714px | Metadata/section-label token |
| Input | Source Code Pro 13px / 450, 34px high, 12px horizontal padding | Inter 15px / 450, 32px high, 10px horizontal padding | Compact code-input variant |
| Textarea | Source Code Pro 13px / 450, 80px high, 6px radius | Inter 14px / 450, 80px high, 10px radius | Compact multiline-input variant |
| Button | Inter 12px / 450, 34px high, 6px radius | Inter 13px / 450, 32px high, mixed page overrides | Shared compact button variants |
| Table body | Inter 13px / 450 / 18.5714px | Inter 14px / 450 / 20px in generic primitive | Shared Dashboard table metrics |
| Borders | off-white at 7.5–13.5% opacity | hard-coded #292d2b/#3a403d plus light Shadcn defaults | One border token hierarchy |
| Shell | 48px top bar, dense fixed icon rail, page-level secondary nav | fixed rail and secondary nav exist but use independent geometry rules | Shell dimensions and responsive offsets |

## Route coverage matrix

| Family | Routes |
| --- | --- |
| Entry | `/login`, `/setup` |
| Projects | `/projects`, `/projects/new`, `/projects/:id/overview` |
| Server settings | `/projects/:id/configuration` |
| Authentication | `/projects/:id/authentication/*` |
| Functions | `/projects/:id/functions`, `/projects/:id/functions/secrets` |
| Manager settings | `/settings` |
| Fallback states | Not found, unavailable Authentication routes, loading/error/empty/operation states |

## Audit method

For each route family, compare title, label, body, metadata, control, table, card, dialog, menu, empty, error, loading, hover, focus, disabled, and mobile-sheet states at 1440px, 1024px, and 375px. Preserve Manager text and behavior while recording the visual completion result in this file.
