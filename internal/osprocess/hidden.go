package osprocess

import "os/exec"

// HideWindow prevents child console windows from flashing when Agent Builder is
// running from a GUI process. It is a no-op on non-Windows platforms.
func HideWindow(cmd *exec.Cmd) {
	hideWindow(cmd)
}
