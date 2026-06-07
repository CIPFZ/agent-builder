Create, overwrite, or append to a file with given content; auto-creates parent dirs. Read the file first to avoid conflicts. For surgical changes use edit or multiedit.

Use `mode: "overwrite"` by default. Use `mode: "append"` for long reports or large generated files, adding one section at a time so each tool call remains small and valid JSON.
