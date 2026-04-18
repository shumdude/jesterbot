# ADR 000: tgaml bootstrap migration

## Status

Accepted for the current migration phase.

## Decision

- `jesterbot` connects `tgaml` through a local module replacement: `replace gobot => ../gobot`.
- The runtime FSM uses `github.com/go-telegram/fsm`.
- For this phase the FSM backend stays in-memory, which preserves the current bot behavior where transport session state is not persisted across restarts.
- The old `internal/telegram.Router` remains as a compatibility bridge for non-migrated UI flows, callback handlers, and scheduler delivery.

## Rationale

- The repository already has a local sibling checkout with `tgaml`, so a local `replace` gives a deterministic import path without waiting for a published module.
- Using `go-telegram/fsm` now aligns the transport layer with the target architecture from `migration.md` while keeping backend complexity small.
- An in-memory FSM backend is a safe first step because the current router already keeps session state in memory; this avoids introducing Redis or a custom SQLite-backed FSM during the same refactor.
- Keeping the legacy router as a notifier and text/callback bridge lets the app move to `tgaml` bootstrap immediately without rewriting every feature in one change.

## Consequences

- Running the project now requires the sibling `../gobot` checkout to be present.
- FSM state is still ephemeral. A later phase should replace the default in-memory backend with a persistent storage implementation if restart-safe conversation state is required.
- The scheduler is now decoupled from the concrete router through a notifier interface, which reduces the remaining migration surface.
