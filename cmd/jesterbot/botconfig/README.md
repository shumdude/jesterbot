# botconfig

Embedded `tgaml` runtime configuration for `jesterbot`.

- `config/messages.yaml` stores user-facing texts and button captions.
- `config/keyboards.yaml` stores inline keyboards used by `tgaml` scenes.
- `config/scenes.yaml` stores FSM scene declarations and command routing.

This package exists so `internal/app` can load the YAML bundle through an importable package instead of depending on the `main` package.
