//go:build webview_test

package main

import (
	"path/filepath"
	"testing"
)

func TestDesktopWebviewTestOptionsRemoteDebugContract(t *testing.T) {
	t.Setenv(webviewTestPortEnv, "9333")
	t.Setenv(webviewTestUserDataEnv, filepath.Join(t.TempDir(), "tmp", "runtime-dev", "phase361", "webview"))

	windowsOptions, windowOptions, err := desktopWebviewTestOptions()
	if err != nil {
		t.Fatalf("desktopWebviewTestOptions returned error: %v", err)
	}
	if windowsOptions.WebviewUserDataPath == "" {
		t.Fatal("expected isolated WebView user-data path")
	}
	if len(windowsOptions.AdditionalBrowserArgs) != 1 || windowsOptions.AdditionalBrowserArgs[0] != "--remote-debugging-port=9333" {
		t.Fatalf("unexpected browser args: %#v", windowsOptions.AdditionalBrowserArgs)
	}
	if !windowOptions.DevToolsEnabled {
		t.Fatal("expected DevTools enabled for test-tagged WebView automation")
	}
	if windowOptions.OpenInspectorOnStartup {
		t.Fatal("inspector should stay closed unless explicitly requested")
	}
}

func TestDesktopWebviewTestOptionsRejectsUnsafeUserDataPath(t *testing.T) {
	t.Setenv(webviewTestPortEnv, "9333")
	t.Setenv(webviewTestUserDataEnv, filepath.Join(t.TempDir(), "webview"))

	if _, _, err := desktopWebviewTestOptions(); err == nil {
		t.Fatal("expected unsafe user-data path to be rejected")
	}
}

func TestDesktopWebviewTestOptionsRejectsInvalidPort(t *testing.T) {
	t.Setenv(webviewTestPortEnv, "not-a-port")
	t.Setenv(webviewTestUserDataEnv, filepath.Join(t.TempDir(), "tmp", "runtime-dev", "phase361", "webview"))

	if _, _, err := desktopWebviewTestOptions(); err == nil {
		t.Fatal("expected invalid remote-debug port to be rejected")
	}
}
