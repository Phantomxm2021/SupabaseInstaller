# Runtime Settings Discoverability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the create-server wizard's collapsed runtime control visibly interactive and clear about its pinned-version setting.

**Architecture:** Keep the existing Radix Collapsible state and version Select. Replace its text-only trigger with a full-width header button that carries title, description, safe version summary, action text, and a stateful chevron. Tests exercise the rendered accessible state rather than internal component state.

**Tech Stack:** React, TypeScript, React Hook Form, Radix Collapsible, Lucide React, Vitest, Testing Library.

---

### Task 1: Specify the closed and open runtime-control contract

**Files:**

- Modify: `apps/web/src/features/projects/NewProjectPage.test.tsx:266-284`
- Test: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] **Step 1: Replace the closed-state assertion with an interaction test**

    const runtimeSettings = screen.getByRole('button', { name: /advanced runtime settings/i })
    expect(runtimeSettings).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('Supabase self-hosted/v0.8.0 · 1 setting')).toBeVisible()
    expect(screen.getByText('Choose the pinned Supabase runtime version for this server.')).toBeVisible()
    expect(screen.getByText('Expand settings')).toBeVisible()
    expect(screen.queryByLabelText('Pinned Supabase version')).not.toBeInTheDocument()
    await user.click(runtimeSettings)
    expect(runtimeSettings).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Hide settings')).toBeVisible()
    expect(screen.getByLabelText('Pinned Supabase version')).toBeVisible()

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `npm --prefix apps/web test -- --run src/features/projects/NewProjectPage.test.tsx`

Expected: FAIL because the current trigger is named `Runtime settings` and has no summary or action text.

### Task 2: Render an explicit, accessible advanced-settings trigger

**Files:**

- Modify: `apps/web/src/features/projects/BasicStep.tsx:1-10,134-146`
- Test: `apps/web/src/features/projects/NewProjectPage.test.tsx`

- [ ] **Step 1: Import the chevron icon**

    import { ChevronDown } from 'lucide-react'

- [ ] **Step 2: Replace the text-only trigger with the full-width header**

    <Collapsible>
      <CollapsibleTrigger className="flex w-full items-center justify-between rounded-lg border border-border px-4 py-3 text-left hover:bg-muted/50">
        <span>Advanced runtime settings, description, and version summary</span>
        <span>Expand or hide state text and ChevronDown</span>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-4">existing version selector</CollapsibleContent>
    </Collapsible>

Use the primitive's `data-state` attributes or a controlled `open` state so action text and icon state always agree with `aria-expanded`.

- [ ] **Step 3: Run the focused test to verify it passes**

Run: `npm --prefix apps/web test -- --run src/features/projects/NewProjectPage.test.tsx`

Expected: PASS, including the runtime-control interaction test.

- [ ] **Step 4: Run Web verification and inspect formatting**

Run: `npm --prefix apps/web test -- --run && npm --prefix apps/web run build && git diff --check`

Expected: all Web tests and production build pass; no whitespace errors.

- [ ] **Step 5: Commit the implementation**

Run: `git add apps/web/src/features/projects/BasicStep.tsx apps/web/src/features/projects/NewProjectPage.test.tsx && git commit -m 'feat(wizard): clarify runtime settings'`
