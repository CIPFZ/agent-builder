---
name: skill-creator
description: Use when creating, editing, or validating Crush skills, including builtin skills, project skills, and skill discovery checks.
---

# Skill Creator

Use this skill when you need to author or revise a Crush skill.

## What To Do

- Keep the skill focused on one workflow.
- Write a clear description that explains when to use it.
- Include concise steps and concrete examples.
- Point to the repo paths or validation checks that matter.

## Skill Structure

1. Start with valid frontmatter.
2. Make the directory name match the `name` field.
3. Write short, direct instructions.
4. Add examples only when they remove ambiguity.
5. Include validation notes if the skill changes files, config, or tests.

## Validation

- Parse the file as `SKILL.md`.
- Confirm the skill name matches its folder.
- Check discovery after adding or editing the skill.
- Verify the instructions still match Crush behavior.

## Examples

To add a new skill:

1. Create a folder under the configured skills path.
2. Write `SKILL.md` with frontmatter and instructions.
3. Refresh skill discovery.
4. Inspect the skill state and fix validation errors.

To update a builtin skill:

1. Edit the builtin `SKILL.md`.
2. Update related tests if the name or path changed.
3. Re-run discovery and any relevant runtime smoke tests.
