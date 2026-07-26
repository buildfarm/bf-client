package view

import (
	"image"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/buildfarm/bf-client/client"
)

type message struct {
	ui []ui.Drawable
	v View
}

func minRect(x int, y int, l int) image.Rectangle {
	// single line paragraph dims for SetRect params
	return image.Rect(
		x - l / 2 - 1, y - 1,
		x + (l + 1) / 2 + 1, y + 2)
}

func NewMessage(s string, c client.Component, v View) View {
	dims := c.Dimensions()
	p := widgets.NewParagraph()
	// compute required size for box
	sl := len(s)
	midx, midy := dims.Width / 2, dims.Height / 2
	r := minRect(midx, midy, sl)
	p.SetRect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
	p.Text = s

	return &message {
		ui: []ui.Drawable { p },
		v: v,
	}
}

func (m *message) Update() {
}

func (m message) Render() []ui.Drawable {
	return m.ui
}

func (m *message) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>", "<Enter>", " ", "<Tab>":
		return m.v
	}
	return m
}
