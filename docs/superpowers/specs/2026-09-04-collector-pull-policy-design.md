# Function Log Collector Pull Policy Design

## Problem

Official runtime synchronization executes `docker compose pull` for the rendered project. The Function log collector uses the locally built `supabase-provisioner:<tag>` image, so Compose incorrectly attempts to pull it from a registry and aborts the entire official-image pull.

## Design

Render `pull_policy: never` only on `function-log-collector`. The collector must reuse the concrete Provisioner image built and selected by the Manager installer. All official Supabase services retain their existing pull behavior. If the local Provisioner image is missing, Compose must fail rather than silently retrieving an unrelated registry image.

## Verification

Assert the rendered collector service has `pull_policy: never`, official services do not inherit it, and rendered Compose validates with Docker Compose. Run render, runtime reconcile, and full Go tests.
