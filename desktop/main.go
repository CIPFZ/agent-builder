package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	webviewTestOptions, webviewTestWindowOptions, err := desktopWebviewTestOptions()
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "Agent Builder",
		Description: "Agentic operations desktop client",
		Windows:     webviewTestOptions,
		Services: []application.Service{
			application.NewService(NewRuntimeBridge()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	windowOptions := application.WebviewWindowOptions{
		Title:            "Agent Builder",
		Width:            1180,
		Height:           820,
		Frameless:        false,
		MinWidth:         1040,
		MinHeight:        760,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	}
	applyDesktopWebviewTestWindowOptions(&windowOptions, webviewTestWindowOptions)
	app.Window.NewWithOptions(windowOptions)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
