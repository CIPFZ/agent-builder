---
name: skill-creator
description: Use when creating, editing, evaluating, or validating Crush skills, including builtin skills, project skills, and skill discovery checks.
---

# Skill Creator

Use this skill when the user wants to create a new skill, improve an existing
skill, or verify that skills are discoverable and useful in Crush.

This skill adapts the Anthropic skill-creator workflow for Crush. Keep skills
small, behavior-oriented, and testable. A skill should teach the agent how to do
one kind of work better; it should not become a generic documentation dump.

## Skill Design

1. Identify the workflow the skill should improve.
2. Write the `description` as a trigger: "Use when...".
3. Keep instructions concrete and procedural.
4. Include only references, scripts, or examples that the agent should actually
   use.
5. Avoid secrets, local machine assumptions, and stale external links.
6. Prefer repository-native checks over manual inspection.

Good skills are narrow enough that the trigger is obvious. If a skill would
need many unrelated sections, split it into multiple skills.

## Crush Skill Structure

Each Crush skill is a directory containing `SKILL.md`.

```text
<skills-path>/
  my-skill/
    SKILL.md
```

`SKILL.md` must start with YAML frontmatter:

```markdown
---
name: my-skill
description: Use when the user needs a focused workflow.
---

# My Skill

Follow these steps...
```

The directory name should match the `name` field. Builtin skills live under
`internal/skills/builtin/<skill-name>/SKILL.md`. Project skills normally live
under `.agents/skills/<skill-name>/SKILL.md` or another configured
`options.skills_paths` entry.

## Authoring Workflow

1. Create or edit the skill directory.
2. Keep the top-level heading human-readable.
3. Put activation guidance in `description`, not only in the body.
4. Put operational steps in the body.
5. Add validation steps that can run locally.
6. Refresh runtime skill discovery after editing.
7. Confirm enabled/disabled state comes from runtime config, not frontend state.

When adding a builtin skill, also update or add assertions in
`internal/skills/skills_test.go`.

## Evaluation Checklist

Before considering a skill done, verify:

- The frontmatter parses.
- The skill name is valid and matches its directory.
- The description clearly says when to use it.
- The instructions are short enough to be read during context assembly.
- Builtin discovery exposes `crush://skills/<name>` paths.
- User skills with the same name override builtin skills.
- Disabling the skill through runtime config excludes it from agent context.

Use these checks when relevant:

```text
go test ./internal/skills
go test ./desktop/agent-builder
```

For desktop/runtime validation, refresh skills from Runtime Details and confirm
the skill appears in both Skills and Capabilities.

## Editing Existing Skills

When revising a skill:

1. Preserve the existing `name` unless a rename is explicitly intended.
2. Keep user-facing behavior compatible where possible.
3. Remove outdated steps instead of appending corrections.
4. Re-run discovery tests.
5. If runtime UI behavior changes, run the desktop smoke path.

## Anti-Patterns

- Broad "general coding" skills.
- Instructions that duplicate AGENTS.md or global system behavior.
- Hidden state owned only by the frontend.
- Long essays with no actionable steps.
- References to scripts or assets that are not present in the skill directory.
