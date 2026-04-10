# internal/config

Configuration schema and loading logic.

Includes environment parsing and default handling for startup configuration.

User-facing reminder behavior and per-user scheduler check frequency live in persisted bot settings; the global scheduler loop itself remains an internal fixed-minute runtime detail rather than an env knob.

The env surface is intentionally small: bot token, DB path, and poll timeout.
