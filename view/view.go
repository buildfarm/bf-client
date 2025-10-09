package view

import (
	ui "github.com/gizak/termui/v3"
)

type View interface {
	Handle(ui.Event) View
	Update()
	Render() []ui.Drawable
}
