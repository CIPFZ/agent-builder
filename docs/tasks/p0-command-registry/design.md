# P0 Command Registry Design

Commands are runtime capabilities with stable metadata and execution results.

Contracts:

- Registered slash commands have canonical names, aliases, descriptions, argument hints, category, and visibility.
- Visibility depends on runtime state and permission mode through a shared context.
- Execution can return immediate output or request continuation into model query.
- TUI and other clients list and resolve commands through the same registry.