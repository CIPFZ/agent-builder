//go:build !windows

package osprocess

import "os/exec"

func hideWindow(*exec.Cmd) {}
