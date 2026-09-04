# Multiline Function Environment Values Design

## Problem

The Edge Function secrets UI promises support for multiline PEM, JSON, and function values, but Provisioner rejects every Unicode control character before rendering `.env`. This incorrectly rejects LF and CRLF in valid PEM private keys.

## Design

Keep the existing encrypted secret flow and dotenv renderer. Permit horizontal tab, line feed, and carriage return in Function environment values because `escapeDotEnv` already quotes and escapes them with `strconv.Quote`. Continue rejecting all other Unicode/C1 control characters, including NUL. Apply this exception only to the Function-specific environment map; the shared runtime environment validation remains strict.

Normalize neither line endings nor content so the function receives the value the user supplied after dotenv decoding.

## Verification

Add render tests proving a multiline PEM with LF and CRLF renders successfully and round-trips as an escaped dotenv value. Add negative coverage proving NUL remains rejected. Run the render package tests and the full Go suite.
