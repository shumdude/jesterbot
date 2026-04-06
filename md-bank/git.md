Git workflow for this repo:

1. Do not commit or push unless the user explicitly asks.
2. Re-read `.codex/agents/git_committer.toml` before writing a commit.
3. Stage only files that belong to the requested task or current phase.
4. Leave unrelated worktree changes alone.
5. Use Conventional Commits:
   `<type>[optional scope]: <description>`
6. Keep the subject short, imperative, and lowercase for type and scope.
7. Add a body when it helps explain what changed and why.
8. Before commit, report the exact checks that already ran and their result.
9. If the phase contains unrelated docs and code changes, prefer separate commits.

Practical reminder:

- Check `git status --short` before staging.
- Prefer `git add <file...>` over broad staging.
- Review the staged diff before commit.
- Do not use destructive git commands unless the user explicitly asks.
