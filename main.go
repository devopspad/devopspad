package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Infradex",
		Width:  1480,
		Height: 940,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		MinWidth:         1000,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 250, G: 251, B: 252, A: 255},
		OnShutdown:       app.shutdown,
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
