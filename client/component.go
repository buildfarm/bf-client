package client

import (
	ui "github.com/gizak/termui/v3"
)

type Component interface {
	Open(args[] string)
	Close()
	Run() Component
	App() *App

	// viewComponent?
	Finish()
	Dimensions() ui.Resize // Screen?
}
