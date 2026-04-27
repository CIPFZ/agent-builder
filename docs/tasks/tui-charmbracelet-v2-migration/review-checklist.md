# TUI Charmbracelet v2 Migration Review Checklist

Date: 2026-04-25

Use this checklist when reviewing the migration branch.

## 1. Dependency Check

- Does `go.mod` use `charm.land/bubbletea/v2`?
- Does `go.mod` use `charm.land/lipgloss/v2`?
- Are legacy Charmbracelet direct imports removed from `internal/tui`?

## 2. Architecture Boundary Check

- Does production TUI still use `myclawd`?
- Was no production `RuntimeBridge` or direct runtime path reintroduced?

## 3. Behavior Check

- Keyboard input still works.
- Mouse wheel scrolling still works.
- Window resize still works.
- Alt screen startup/shutdown still works.
- Lip Gloss width/layout behavior remains correct.

## 4. Scope Check

- Did the branch stay focused on migration?
- Did it avoid broad TUI feature expansion?

## 5. Merge Blockers

Do not approve if:

- production TUI still imports legacy Charmbracelet paths
- TUI bypasses `myclawd`
- tests fail
- terminal lifecycle behavior regresses
