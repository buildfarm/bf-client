package view

import (
	ui "github.com/gizak/termui/v3"
)

type View interface {
	Handle(ui.Event) View
	Update()
	Render() []ui.Drawable
}

func UISize() ui.Resize {
	var size ui.Resize
	size.Width, size.Height = ui.TerminalDimensions()
	return size
}
