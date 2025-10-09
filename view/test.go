package view

import (
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/werkt/bf-client/client"
)

type testView struct {
	a       *client.App
	v       View
	console *widgets.Paragraph
}

func NewTest(a *client.App, v View) View {
	console := widgets.NewParagraph()
	console.SetRect(0, 0, 120, 40)
	console.Title = "Console"
	console.WrapText = true
	return &testView{
		a:       a,
		v:       v,
		console: console,
	}
}

func (v *testView) Render() []ui.Drawable {
	return []ui.Drawable{v.console}
}

func (v *testView) Handle(e ui.Event) View {
	if e.ID == "<C-c>" {
		ui.Clear()
		return v.v
	}

	v.console.Text += "\n" + e.ID
	return v
}

func (v *testView) Update() {
}
