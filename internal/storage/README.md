# internal/storage

Persistence layer abstractions and concrete backends.

- `sqlite/`: SQLite schema, migrations, and repository implementation.
- Production rule: schema changes are migration-only. Do not change DB structure directly.
