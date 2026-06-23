---
name: upstream-release-merge
description:
  Use when comparing, evaluating, or merging a new Charm Agent Builder upstream release
  into Agent Builder, especially when the user mentions upstream Agent Builder versions,
  release tags, upstream-stable, v0.x.x, or keeping the client project aligned
  with Agent Builder without reintroducing CLI/TUI-heavy changes.
---

# Upstream Release Merge

Use this workflow to bring useful changes from Charm Agent Builder into Agent Builder
while preserving Agent Builder's client-first architecture.

## Ground Rules

- Treat Agent Builder as a desktop client project built on Agent Builder runtime code,
  not as a full Agent Builder fork.
- Preserve the original Agent Builder license and upstream lineage.
- Prefer small, traceable upstream commits over a bulk merge.
- Use `cherry-pick -x` when applying upstream commits so future audits can
  trace the source commit.
- Do not reintroduce upstream README, release automation, packaging, Nix,
  legacy CI, CLA, or `.github/workflows` content unless the user explicitly
  requests it.
- Do not merge TUI-only work by default. TUI code may be kept temporarily, but
  Agent Builder's target surface is React + Wails + Go runtime.
- Keep local user changes. Never reset or revert unrelated work.

## Required Inputs

Identify these before changing files:

- Current project branch, usually `main`.
- Current upstream baseline commit already absorbed by Agent Builder.
- Target upstream release tag or release commit.
- Whether `origin/upstream-stable` is available from the sync workflow.

If the user provides a baseline, use it. Otherwise inspect:

```bash
git log --oneline --decorate --all --grep "v0."
git branch --all --verbose --no-abbrev
git log --oneline origin/upstream-stable -20
```

## Fetch Upstream Context

Run from the repository root:

```bash
git status --short --branch
git remote -v
git fetch origin upstream-stable:refs/remotes/origin/upstream-stable --prune
git fetch upstream --tags --prune
```

If tag fetching is slow or unavailable, use `origin/upstream-stable` and inspect
the inner upstream release commit. The sync branch may contain one local commit
that strips upstream workflows; do not treat that wrapper commit as the upstream
release itself.

## Difference Analysis

Compare the known baseline with the target upstream release:

```bash
git log --oneline --no-merges <baseline>..<target>
git diff --stat <baseline> <target>
git diff --name-status <baseline> <target>
```

Group upstream changes into these buckets:

- **Client/runtime valuable**: server, runtime API, workspace, config, provider,
  auth, MCP, hooks, message protocol, tool behavior, DB, permissions, security,
  stability, tests for these areas.
- **Desktop-client relevant**: changes that improve client-server mode,
  event streaming, session state, todo state, OAuth, provider discovery, or
  runtime reliability.
- **CLI/TUI-only**: Bubble Tea rendering, terminal layout, terminal markdown,
  terminal input, terminal-only dialogs, terminal packaging.
- **Upstream repo maintenance**: CLA signatures, upstream workflows,
  release/snapshot/nightly automation, docs for installing Agent Builder CLI,
  package-manager metadata, Nix, labels, Dependabot.
- **Risky broad mechanical changes**: large gofumpt-only sweeps or generated
  docs mixed with many unrelated files.

Default selection:

- Apply client/runtime valuable and desktop-client relevant changes.
- Skip CLI/TUI-only and upstream repo maintenance changes.
- Apply provider and dependency updates only when they are needed by selected
  runtime commits or are low-risk.
- Keep generated swagger updates only if selected API/apitypes changes require
  them.

## Applying Changes

Prefer cherry-picking individual upstream commits:

```bash
git cherry-pick -x <commit1> <commit2> ...
```

If a commit mixes useful runtime code with unwanted README or workflow changes:

1. Resolve conflicts by preserving Agent Builder files for project identity.
2. Keep the runtime/runtime parts.
3. Drop upstream CLI docs and repository automation.
4. Continue the cherry-pick.

Useful conflict patterns:

```bash
git checkout --ours README.md
git add README.md
git cherry-pick --continue
```

Use this only after confirming the conflict is upstream CLI documentation
overwriting Agent Builder documentation.

If a cherry-pick is too mixed, abort that commit and manually port the minimal
code changes:

```bash
git cherry-pick --abort
git show <commit> -- <paths>
```

Then apply only the relevant paths with normal edits.

## Validation

Run the same lightweight checks used by daily CI:

```bash
gofmt -l $(git ls-files '*.go' ':!:desktop/build/**')
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -failfast -skip '^TestCoderAgent$' . ./internal/...
```

Notes:

- `TestCoderAgent` is a recorded LLM interaction test and may fail when prompts
  or tool descriptions change. Do not use it as a daily merge gate unless the
  cassette is intentionally regenerated.
- If `go mod tidy` fails due a transient Go proxy EOF, retry once before
  treating it as a code problem.
- Do not run full Wails desktop builds unless the selected upstream changes
  touch `desktop/`, `client/`, frontend assets, Wails config, or release logic.

## Commit And Push Rules

- Keep upstream cherry-pick commits separate when they are already clean and
  semantic.
- Use one extra local commit only for Agent Builder-specific adaptations.
- Commit messages must remain semantic.
- Push only when the user asked for commit/push or the current task explicitly
  includes submitting the merge.

Recommended final report:

- Baseline and target release used.
- Upstream commits merged.
- Upstream commits or categories skipped, with reasons.
- Conflicts resolved and any Agent Builder-specific choices.
- Validation commands and results.
- Push status.

## Example Decision

For a Agent Builder `v0.69.0` to `v0.70.0` update:

- Merge server panic recovery, network retry dependency updates,
  client-server OAuth support, event/todo state fixes, provider additions, and
  SSE hot-path logging improvements.
- Skip fast TUI rendering/cache refactors unless the project still depends on
  that TUI path.
- Preserve Agent Builder README instead of upstream Agent Builder installation docs.
