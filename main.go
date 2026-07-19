package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:                    "bitcoin++ videos",
		Width:                    1280,
		Height:                   820,
		MinWidth:                 1000,
		MinHeight:                680,
		AssetServer:              &assetserver.Options{Assets: assets},
		BackgroundColour:         &options.RGBA{R: 246, G: 242, B: 234, A: 1},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		Bind:                     []interface{}{app},
		DragAndDrop:              &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: false},
		EnableDefaultContextMenu: false,
	})
	if err != nil {
		log.Fatal(err)
	}
}
