package main

import (
	"embed"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, "--startup") {
			app.startHidden = true
		}
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:            "NeoBot Desktop",
		Width:            1024,
		Height:           768,
		MinWidth:         720,
		MinHeight:        560,
		Frameless:        true,
		StartHidden:      app.startHidden,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 232, G: 243, B: 253, A: 255},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "neobot-desktop",
			OnSecondInstanceLaunch: app.secondInstanceLaunch,
		},
		OnStartup:     app.startup,
		OnDomReady:    app.domReady,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
