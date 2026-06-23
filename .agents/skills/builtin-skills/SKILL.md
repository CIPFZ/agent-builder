---
name: builtin-skills
description:
  Use when creating a new builtin skill for Agent Builder, editing an existing builtin
  skill (internal/skills/builtin/), or when the user needs to understand how the
  embedded skill system works.
---

# Builtin Skills

Agent Builder embeds skills directly into the binary via `internal/skills/builtin/`.
These are always available without user configuration.

## How It Works

- Each skill lives in `internal/skills/builtin/<skill-name>/SKILL.md`.
- The tree is embedded at compile time via `//go:embed builtin/*` in
  `internal/skills/embed.go`.
- `DiscoverBuiltin()` walks the embedded FS, parses each `SKILL.md`, and sets
  paths with the `agent-builder://skills/` prefix (e.g., `agent-builder://skills/jq/SKILL.md`).
- The View tool resolves `agent-builder://` paths from the embedded FS, not disk.
- User skills with the same name override builtins (last occurrence wins in
  `Deduplicate()`).

## Adding a New Builtin Skill

1. Create `internal/skills/builtin/<skill-name>/SKILL.md` with YAML frontmatter
   (`name`, `description`) and markdown instructions. The directory name must
   match the `name` field.
2. No extra wiring needed — `//go:embed builtin/*` picks up new directories
   automatically.
3. Add a test assertion in `TestDiscoverBuiltin` in
   `internal/skills/skills_test.go` to verify discovery.
4. Build and test: `go build . && go test ./internal/skills/...`

## Existing Builtin Skills

| Skill          | Directory               | Description                                |
| -------------- | ----------------------- | ------------------------------------------ |
| `agent-builder-config` | `builtin/agent-builder-config/` | Agent Builder configuration help                   |
| `agent-builder-hooks`  | `builtin/agent-builder-hooks/`  | Authoring, configuring and debugging hooks |
| `jq`           | `builtin/jq/`           | jq JSON processor usage guide              |
