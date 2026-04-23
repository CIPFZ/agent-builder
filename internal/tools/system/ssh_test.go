package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"myclaw/internal/tools"
)

type fakeSSHExecutor struct {
	result SSHResult
	err    error
	input  SSHInput
}

func (f *fakeSSHExecutor) Execute(ctx context.Context, input SSHInput, progressFn func(tools.ToolProgress)) (SSHResult, error) {
	f.input = input
	progressFn(tools.ToolProgress{Type: "ssh.started", Message: "start", Data: map[string]any{}})
	if f.err != nil {
		progressFn(tools.ToolProgress{Type: "ssh.failed", Message: "failed", Data: map[string]any{}})
		return f.result, f.err
	}
	progressFn(tools.ToolProgress{Type: "ssh.finished", Message: "done", Data: map[string]any{"exit_code": f.result.ExitCode}})
	return f.result, nil
}

func TestParseSSHInput_ValidRequiredFields(t *testing.T) {
	input, err := parseSSHInput(`{"host":"example.com","command":"pwd"}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.Host != "example.com" {
		t.Fatalf("expected host example.com, got %q", input.Host)
	}
	if input.Command != "pwd" {
		t.Fatalf("expected command pwd, got %q", input.Command)
	}
}

func TestParseSSHInput_OptionalFieldsAndTrim(t *testing.T) {
	input, err := parseSSHInput(`{
		"host": "  test.local  ",
		"command": "  ls -la  ",
		"user": "  dev  ",
		"port": 2222,
		"timeout": 1500,
		"workdir": "  /tmp/app  ",
		"identity_file": "  /home/user/.ssh/id_rsa  "
	}`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.Host != "test.local" || input.Command != "ls -la" || input.User != "dev" {
		t.Fatalf("unexpected trimmed fields: %+v", input)
	}
	if input.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", input.Port)
	}
	if input.Timeout != 1500*time.Millisecond {
		t.Fatalf("expected timeout 1500ms, got %v", input.Timeout)
	}
	if input.Workdir != "/tmp/app" || input.IdentityFile != "/home/user/.ssh/id_rsa" {
		t.Fatalf("unexpected optional fields: %+v", input)
	}
}

func TestParseSSHInput_RejectsPlainString(t *testing.T) {
	_, err := parseSSHInput("ssh host pwd")
	if err == nil {
		t.Fatal("expected error for plain string input")
	}
}

func TestParseSSHInput_RejectsMalformedJSON(t *testing.T) {
	_, err := parseSSHInput(`{"host":"x",`)
	if err == nil {
		t.Fatal("expected error for malformed json")
	}
}

func TestParseSSHInput_RejectsMissingRequired(t *testing.T) {
	_, err := parseSSHInput(`{"host":"example.com"}`)
	if err == nil || !strings.Contains(err.Error(), "requires command") {
		t.Fatalf("expected missing command error, got %v", err)
	}
	_, err = parseSSHInput(`{"command":"pwd"}`)
	if err == nil || !strings.Contains(err.Error(), "requires host") {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestSSHToolDefinition(t *testing.T) {
	tool := NewSSHTool(nil)
	def := tool.Definition()
	if def.Name != "SSH" {
		t.Fatalf("expected name SSH, got %q", def.Name)
	}
	required, ok := def.InputSchema["required"].([]string)
	if !ok {
		anyRequired, okAny := def.InputSchema["required"].([]any)
		if !okAny {
			t.Fatalf("required field has unexpected type: %T", def.InputSchema["required"])
		}
		required = make([]string, 0, len(anyRequired))
		for _, v := range anyRequired {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("required entry has unexpected type: %T", v)
			}
			required = append(required, s)
		}
	}
	joined := strings.Join(required, ",")
	if !strings.Contains(joined, "host") || !strings.Contains(joined, "command") {
		t.Fatalf("required fields missing host/command: %v", required)
	}
	if tool.IsReadOnly(`{"host":"x","command":"pwd"}`) {
		t.Fatal("SSH must not be read-only in v1")
	}
	if !tool.IsDestructive(`{"host":"x","command":"pwd"}`) {
		t.Fatal("SSH must be destructive in v1")
	}
}

func TestBuildSSHArgs(t *testing.T) {
	args := buildSSHArgs(SSHInput{Host: "example.com", Command: "pwd"})
	if len(args) == 0 || args[len(args)-1] != "example.com" {
		t.Fatalf("unexpected host-only args: %v", args)
	}

	args = buildSSHArgs(SSHInput{Host: "example.com", User: "dev", Port: 2222, IdentityFile: "/tmp/id", Command: "pwd"})
	joined := strings.Join(args, " ")
	checks := []string{"-p 2222", "-i /tmp/id", "dev@example.com"}
	for _, want := range checks {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in args %q", want, joined)
		}
	}
}

func TestBuildRemoteCommand(t *testing.T) {
	command := buildRemoteCommand(SSHInput{Command: "ls", Workdir: "/tmp/app"})
	if command != "cd '/tmp/app' && ls" {
		t.Fatalf("unexpected wrapped command: %q", command)
	}
	command = buildRemoteCommand(SSHInput{Command: "pwd"})
	if command != "pwd" {
		t.Fatalf("unexpected command without workdir: %q", command)
	}
}

func TestSSHToolInvokeWithContext_ResultAndErrorShaping(t *testing.T) {
	f := &fakeSSHExecutor{result: SSHResult{Host: "example.com", Command: "pwd", ExitCode: 2, Stdout: "", Stderr: "bad", DurationMs: 12}, err: errors.New("exit 2")}
	tool := NewSSHTool(nil)
	tool.executor = f

	var progress []tools.ToolProgress
	result, err := tool.InvokeWithContext(context.Background(), tools.ToolUseContext{
		Input:     `{"host":"example.com","command":"pwd"}`,
		ToolUseID: "u1",
		ReportProgress: func(p tools.ToolProgress) {
			progress = append(progress, p)
		},
	})
	if err == nil {
		t.Fatal("expected executor error")
	}
	if result.Output == "" {
		t.Fatal("expected output text")
	}
	if result.StructuredContent == nil {
		t.Fatal("expected structured content")
	}
	structuredJSON, _ := json.Marshal(result.StructuredContent)
	if !strings.Contains(string(structuredJSON), `"host":"example.com"`) {
		t.Fatalf("expected structured host in content, got %s", string(structuredJSON))
	}
	if result.Meta == nil || result.Meta["host"] != "example.com" || result.Meta["command"] != "pwd" {
		t.Fatalf("expected host+command in meta, got %#v", result.Meta)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	for _, p := range progress {
		if p.ToolUseID != "u1" {
			t.Fatalf("expected toolUseID u1, got %q", p.ToolUseID)
		}
		if p.Data["host"] != "example.com" || p.Data["command"] != "pwd" {
			t.Fatalf("expected host/command context in progress data, got %#v", p.Data)
		}
	}
}
