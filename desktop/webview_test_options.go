//go:build webview_test

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	webviewTestPortEnv         = "AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT"
	webviewTestUserDataEnv     = "AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR"
	webviewTestOpenDevtoolsEnv = "AGENT_BUILDER_WEBVIEW_TEST_OPEN_DEVTOOLS"
)

func init() {
	desktopMemoryGuardTreeHighWaterBytes = positiveInt64Env("AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_TREE_BYTES", desktopMemoryGuardTreeHighWaterBytes)
	desktopMemoryGuardWebViewHighWaterBytes = positiveInt64Env("AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_WEBVIEW_BYTES", desktopMemoryGuardWebViewHighWaterBytes)
	desktopMemoryGuardRequiredSamples = int(positiveInt64Env("AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_REQUIRED_SAMPLES", int64(desktopMemoryGuardRequiredSamples)))
	desktopMemoryGuardSampleInterval = time.Duration(positiveInt64Env("AGENT_BUILDER_WEBVIEW_TEST_MEMORY_GUARD_INTERVAL_MS", desktopMemoryGuardSampleInterval.Milliseconds())) * time.Millisecond
}

func positiveInt64Env(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func desktopWebviewTestOptions() (application.WindowsOptions, application.WebviewWindowOptions, error) {
	port := strings.TrimSpace(os.Getenv(webviewTestPortEnv))
	if port == "" {
		return application.WindowsOptions{}, application.WebviewWindowOptions{}, nil
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return application.WindowsOptions{}, application.WebviewWindowOptions{}, fmt.Errorf("%s must be a TCP port between 1 and 65535", webviewTestPortEnv)
	}

	userDataPath := strings.TrimSpace(os.Getenv(webviewTestUserDataEnv))
	if userDataPath == "" {
		return application.WindowsOptions{}, application.WebviewWindowOptions{}, fmt.Errorf("%s is required when %s is set", webviewTestUserDataEnv, webviewTestPortEnv)
	}
	userDataPath, err = filepath.Abs(userDataPath)
	if err != nil {
		return application.WindowsOptions{}, application.WebviewWindowOptions{}, fmt.Errorf("resolve %s: %w", webviewTestUserDataEnv, err)
	}
	if !pathIsUnderRuntimeDev(userDataPath) {
		return application.WindowsOptions{}, application.WebviewWindowOptions{}, fmt.Errorf("%s must be under tmp/runtime-dev", webviewTestUserDataEnv)
	}

	windowOptions := application.WebviewWindowOptions{
		DevToolsEnabled: true,
	}
	if strings.TrimSpace(os.Getenv(webviewTestOpenDevtoolsEnv)) == "1" {
		windowOptions.OpenInspectorOnStartup = true
	}

	return application.WindowsOptions{
		WebviewUserDataPath: userDataPath,
		AdditionalBrowserArgs: []string{
			fmt.Sprintf("--remote-debugging-port=%d", portNumber),
		},
	}, windowOptions, nil
}

func applyDesktopWebviewTestWindowOptions(target *application.WebviewWindowOptions, testOptions application.WebviewWindowOptions) {
	if testOptions.DevToolsEnabled {
		target.DevToolsEnabled = true
	}
	if testOptions.OpenInspectorOnStartup {
		target.OpenInspectorOnStartup = true
	}
}

func pathIsUnderRuntimeDev(path string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(filepath.ToSlash(clean), "/")
	for i := 0; i+1 < len(parts); i++ {
		if strings.EqualFold(parts[i], "tmp") && strings.EqualFold(parts[i+1], "runtime-dev") {
			return true
		}
	}
	return false
}
