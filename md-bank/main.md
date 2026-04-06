Follow these files in this order:

1. `@workflow.md` - the main work loop.
2. `@pitfalls.md` - the common ways agents fail.
3. `@filesystem.md` - how to read and edit files safely.
4. `@folders.md` - where new code and folders should go.
5. `@context7.md` - how to work with external libraries.
6. `@golang.md` - rules for Go code.
7. `@tests.md` - how to prove the work is really done.
8. `@git.md` - how to stage and commit changes safely.
9. `@read.md` - how to write README.md files.

Short version:

- Work in phases. One phase should touch at most 5 files.
- If the task is large, remove dead code and noise first.
- Re-read every file before editing it. Re-read it again after editing.
- Read large files in chunks, not in one pass.
- Do not say the task is done without a real check.
- Do not commit without explicit user instruction. When asked to commit, read `@git.md`.
