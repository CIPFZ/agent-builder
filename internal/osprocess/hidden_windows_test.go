package osprocess

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHideWindowSetsWindowsFlags(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("cmd", "/c", "exit", "0")
	HideWindow(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow is false")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("CREATE_NO_WINDOW was not set")
	}
}

func TestHideWindowPreservesExistingWindowsFlags(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("cmd", "/c", "exit", "0")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS,
	}

	HideWindow(cmd)

	if cmd.SysProcAttr.CreationFlags&windows.DETACHED_PROCESS == 0 {
		t.Fatal("existing flags were not preserved")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("CREATE_NO_WINDOW was not set")
	}
}
