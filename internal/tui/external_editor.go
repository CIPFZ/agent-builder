package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleExternalEditorKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.externalEditor.Active {
		return nil, true
	}
	if m.externalEditor.PendingCtrlX {
		m.externalEditor.PendingCtrlX = false
		if msg.Type == tea.KeyCtrlE {
			return m.startExternalEditor(), true
		}
	}
	switch msg.Type {
	case tea.KeyCtrlG:
		return m.startExternalEditor(), true
	case tea.KeyCtrlX:
		m.externalEditor.PendingCtrlX = true
		m.clearSuggestions()
		return nil, true
	default:
		return nil, false
	}
}

func (m *Model) startExternalEditor() tea.Cmd {
	if m.externalEditor.PromptEditor == nil {
		m.applyExternalEditorError(fmt.Errorf("external editor is not configured"))
		return nil
	}
	m.externalEditor.Active = true
	m.externalEditor.PendingCtrlX = false
	m.clearSuggestions()
	m.historyIndex = -1
	return m.externalEditor.PromptEditor(externalEditorRequest{
		Prompt: m.pastes.expandReferences(m.input),
	})
}

func (m *Model) applyExternalEditorFinished(msg externalEditorFinishedMsg) {
	m.externalEditor.Active = false
	m.externalEditor.PendingCtrlX = false
	if msg.Err != nil {
		m.applyExternalEditorError(msg.Err)
		return
	}
	content := trimSingleTrailingNewline(msg.Content)
	content = m.pastes.recollapseReferences(content)
	if content == m.input {
		return
	}
	m.input = content
	m.cursorPos = len([]rune(m.input))
	m.historyIndex = -1
	m.clearSuggestions()
}

func (m *Model) applyExternalEditorError(err error) {
	m.diagnostics.LastError = err.Error()
	m.events = appendBoundedEvent(m.events, "external editor failed: "+err.Error(), 200)
}

func trimSingleTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\r\n") && !strings.HasSuffix(text, "\r\n\r\n") {
		return strings.TrimSuffix(text, "\r\n")
	}
	if strings.HasSuffix(text, "\n") && !strings.HasSuffix(text, "\n\n") {
		return strings.TrimSuffix(text, "\n")
	}
	return text
}

func defaultPromptEditor(req externalEditorRequest) tea.Cmd {
	editor := externalEditorCommand()
	if editor == "" {
		return func() tea.Msg {
			return externalEditorFinishedMsg{Err: fmt.Errorf("external editor is not configured")}
		}
	}
	command := &promptEditorCommand{
		editor: editor,
		prompt: req.Prompt,
	}
	return tea.Exec(command, func(err error) tea.Msg {
		if err != nil {
			return externalEditorFinishedMsg{Err: err}
		}
		return externalEditorFinishedMsg{Content: command.content}
	})
}

func defaultFileOpener(path string) error {
	editor := externalEditorCommand()
	if editor == "" {
		return fmt.Errorf("external editor is not configured")
	}
	cmd := commandForEditor(editor, path)
	return cmd.Start()
}

func externalEditorCommand() string {
	if value := strings.TrimSpace(os.Getenv("VISUAL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("EDITOR")); value != "" {
		return value
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

type promptEditorCommand struct {
	editor  string
	prompt  string
	content string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func (c *promptEditorCommand) SetStdin(stdin io.Reader)   { c.stdin = stdin }
func (c *promptEditorCommand) SetStdout(stdout io.Writer) { c.stdout = stdout }
func (c *promptEditorCommand) SetStderr(stderr io.Writer) { c.stderr = stderr }

func (c *promptEditorCommand) Run() error {
	path, err := writePromptTempFile(c.prompt)
	if err != nil {
		return err
	}
	defer os.Remove(path)

	cmd := commandForEditor(c.editor, path)
	cmd.Stdin = c.stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c.content = string(data)
	return nil
}

func writePromptTempFile(prompt string) (string, error) {
	file, err := os.CreateTemp("", "myclaw-prompt-*.md")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.WriteString(prompt); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func commandForEditor(editor string, path string) *exec.Cmd {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return exec.Command(path)
	}
	if runtime.GOOS == "windows" && len(parts) > 1 && strings.EqualFold(parts[0], "start") {
		args := append(parts[1:], filepath.Clean(path))
		return exec.Command("cmd", append([]string{"/C", "start", "/WAIT"}, args...)...)
	}
	args := append(parts[1:], path)
	return exec.Command(parts[0], args...)
}
