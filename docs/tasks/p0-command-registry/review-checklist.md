# P0 Command Registry Review Checklist

- [ ] Command metadata is registered in one package.
- [ ] Aliases resolve to canonical names.
- [ ] Visibility is runtime-context aware.
- [ ] Execution results distinguish immediate output from model continuation.
- [ ] TUI delegates command parsing/listing to the shared registry.
- [ ] Focused workstream tests pass.