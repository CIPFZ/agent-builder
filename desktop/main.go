package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := application.New(application.Options{
		Name:        "Agent Builder",
		Description: "Agentic operations desktop client",
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

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Agent Builder",
		Width:            1180,
		Height:           820,
		Frameless:        false,
		MinWidth:         1040,
		MinHeight:        760,
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(255, 255, 255),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
