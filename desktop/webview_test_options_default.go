//go:build !webview_test

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func desktopWebviewTestOptions() (application.WindowsOptions, application.WebviewWindowOptions, error) {
	return application.WindowsOptions{}, application.WebviewWindowOptions{}, nil
}

func applyDesktopWebviewTestWindowOptions(target *application.WebviewWindowOptions, testOptions application.WebviewWindowOptions) {
	if testOptions.DevToolsEnabled {
		target.DevToolsEnabled = true
	}
	if testOptions.OpenInspectorOnStartup {
		target.OpenInspectorOnStartup = true
	}
}
