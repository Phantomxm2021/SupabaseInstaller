# Authentication Workspace Design

**Date:** 2026-08-27  
**Status:** Approved  
**References:** User-provided Supabase Cloud screenshots; Supabase Auth self-hosted configuration documentation and pinned `self-hosted-v0.8.0` template.

## 1. Goal

Replace the current configuration page's horizontal section tabs with a Supabase Cloud-inspired Authentication workspace. The workspace must use a dual-sidebar desktop layout: the existing project navigation remains on the left and an Authentication-specific navigation appears in the content area. It must surface all authentication capabilities supported by the Manager's pinned self-hosted Supabase Auth runtime through typed, validated settings rather than raw environment-file editing.

## 2. Information Architecture

Authentication routes live beneath `/projects/:projectId/authentication/*`.

The content area contains a fixed-width Authentication navigation and a scrollable main panel. On tablet/mobile the Authentication navigation becomes an accessible collapsible drawer. The global project sidebar is preserved.

Authentication navigation groups:

- **Manage:** Users, OAuth Apps
- **Notifications:** Emails
- **Configuration:** Policies (link to database policy capability), Sign In / Providers, Passkeys, OAuth Server, Sessions, Rate Limits, Multi-Factor, URL Configuration, Attack Protection, Auth Hooks, Audit Logs, Performance

Features that do not yet have the typed Manager schema and runtime renderer are visibly unavailable with an explicit "Not configured in this Manager version" state. They must never appear as active or editable placeholders.

The existing `/configuration?section=...` horizontal `Tabs` UI is removed. General, Services, Storage, Realtime, Functions, Database, Connection Pool, Gateway & Network, and API & Secrets remain reachable from global project navigation. Existing authentication, SMTP, OAuth, and URL settings migrate to their Authentication workspace routes.

## 3. Page Layout and Interactions

### Users

The Users page is a server-paginated table with search, filters, refresh, selection, provider badges, and Add User action. It uses a typed Manager proxy to the GoTrue admin users API; secrets and service-role credentials remain Manager-side. Empty, loading, permission, and API-error states are explicit.

### OAuth Apps

OAuth Apps lists OAuth Server clients with search and filters. When the OAuth server is disabled, it displays an informative status panel and a route to OAuth Server settings. OAuth Server remains clearly marked Experimental.

### Emails

Emails has local `Templates` and `SMTP Settings` tabs only. Templates are clickable rows for confirmation, invitation, magic link/OTP, email change, password reset, and reauthentication. SMTP uses the reference layout: enabled switch, sender details, host, port, username, and write-only password. Template bodies, notification switches, and SMTP configuration are typed configuration values; HTML/body validation and trusted template-variable handling are server-side.

### Sign In / Providers

This page starts with User Signups settings: allow signup, manual linking, anonymous sign-ins, and confirm email. It then lists Auth providers with icon, label, enabled/disabled badge, and disclosure affordance.

Clicking Email, Phone, SAML, Web3, or an OAuth provider opens a right-side Sheet. The sheet owns its form and Save/Cancel actions. OAuth sheets show enabled state, client ID, write-only client secret, provider-specific inputs, and a derived/copyable callback URL. Email sheets include secure email change, secure password change, current-password requirement, password length/rules, and supported email options. Unsaved changes require confirmation before closing.

### Settings pages

Rate Limits, Multi-Factor, URL Configuration, Sessions, Attack Protection, Passkeys, OAuth Server, Auth Hooks, and Performance use a consistent Settings Card: explanatory label/description left, validated control right, and page-level save action. MFA includes TOTP, phone, WebAuthn where supported, plus factor limits. URL Configuration has Site URL and a manageable redirect URL list. Rate limits show units and safe defaults.

Policies route to Database policy management; Auth does not duplicate RLS configuration. Audit Logs is a paginated/filterable read-only view of GoTrue audit records. Experimental labels apply to Passkeys, OAuth Server, and Auth Hooks where the pinned runtime/API declares them experimental.

## 4. Typed Configuration and Runtime Mapping

The Manager expands the existing `AuthConfig` into grouped, versioned typed data: email templates/notifications, signup and provider policy, password/security/CAPTCHA controls, sessions, rate limits, MFA/WebAuthn/passkeys, OAuth Server, hooks, audit-log settings, and observability/performance settings.

Secrets (SMTP password, OAuth client secrets, CAPTCHA secret, hook secrets, SAML private keys) use the existing encrypted secret store with retain/replace/remove semantics and are redacted from GET responses, operation events, browser cache, and logs.

Each field carries validation and restart metadata. The renderer maps only supported fields to the pinned Auth template's `GOTRUE_*` variables, validates the generated Compose model, recreates Auth only, waits for health, verifies `/auth/v1/settings` where applicable, and restores the prior known-good configuration on failure.

Users, OAuth Apps, and Audit Logs use dedicated typed Manager endpoints. They never expose a generic GoTrue proxy or service-role key to the browser.

## 5. Error Handling and Accessibility

Every page supports loading, empty, field-error, authorization-error, mutation-pending, success, and runtime-failure states. Validation is performed client-side for immediate feedback and authoritatively server-side before an operation is queued. Confirmation dialogs state affected services and rollback behavior.

The workspace uses semantic navigation, labelled controls, visible keyboard focus, sheet focus trapping and restoration, keyboard-close behavior, and responsive layouts. Status is always expressed in text as well as color.

## 6. Test Plan

- Unit: typed defaults, field validation, secret redaction, env mapping, special provider fields, MFA/rate-limit/session/password validation.
- Web: dual-sidebar routing and active state, no horizontal configuration tabs, list/filter/empty states, Sheets, unsaved-close confirmation, accessible labels and keyboard interactions.
- Manager/API: pagination/filter contract for Users/OAuth Apps/Audit Logs; authorization; no secret leakage; revision conflicts.
- Provisioner/integration: individual OAuth change, SMTP, MFA, rate limits, sessions, CAPTCHA, and hooks render correct variables, recreate only Auth, verify health, and roll back failed updates.

## 7. Non-goals

The Manager will not copy Supabase Cloud branding or connect to Supabase Cloud's management API. It will not expose raw `.env`, Compose, Docker, or service-role credentials. Database RLS authoring remains a database concern, not an Auth configuration clone.
