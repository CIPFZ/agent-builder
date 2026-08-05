//go:build !windows

package main

import "errors"

type desktopProcessMemory struct {
	ProcessTreePrivateBytes int64
	WebViewPrivateBytes     int64
}

func measureDesktopProcessTreeMemory() (desktopProcessMemory, error) {
	return desktopProcessMemory{}, errors.New("desktop process-tree memory measurement is only available on Windows")
}
