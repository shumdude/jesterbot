# Future Plans

Planned improvements for the repository.

- [x] Add logs to improve runtime visibility and debugging.
- [ ] Add observability with metrics and dashboards such as Prometheus and Grafana.
- [x] Add an i18n mechanism and move bot text into dedicated translation files.
- [x] Add a slider for switching between pages when there are many tasks.
- [x] Stop storing `JESTERBOT_TICK_INTERVAL` in ENV and move scheduler check frequency into persisted bot settings.
- [x] Support batch task creation from a single message separated by `,` or new lines.
- [x] Add one-off tasks as a separate flow and entity that does not intersect with regular recurring tasks.
- [x] Add checkbox sub-items for one-off tasks.
- [x] Make one-off tasks non-recurring after completion.
- [x] Add three priority levels for one-off tasks: low (green), medium (yellow), and high (red).
- [x] Make one-off task reminder frequency configurable for each priority level.
- [x] Use distinct reminder presentation for one-off tasks.
- [x] Include one-off tasks in statistics.
- [x] Use more emojis.
- [x] Write more code comments to make AI-agent-driven development easier.
- [x] Write `README.md` files in project folders.
- [x] Allow regular activities to be completed multiple times per day (e.g. brush teeth 2x).
- [x] Configure reminder time windows for regular activities (e.g. only remind about brushing teeth in the morning and evening, not throughout the day). Default: no window restriction (current behaviour).
