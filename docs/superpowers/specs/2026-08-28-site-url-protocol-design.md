# Fixed HTTPS Site URL Design

## Goal

Make the create-project form derive Site URL from a domain entered by the user, with `https://` fixed by the application instead of typed by the user.

## Interface

The Basic step keeps the existing two-column layout. The Domain control remains a hostname-only text field. The Site URL control displays a static `https://` prefix adjacent to an editable hostname input. Users edit only the hostname portion.

## Data flow

`configuration.general.siteUrl` remains the persisted and API-facing full absolute URL required by the backend. The Basic step watches the editable hostname and writes `https://<hostname>` into that field. Empty input maps to an empty Site URL so normal required-field validation remains authoritative.

## Verification

The page test fills a protocol-free hostname and asserts the project request contains the complete `https://` URL. It also checks that the fixed prefix is visible.
