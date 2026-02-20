package view

import (
	"github.com/gizak/termui/v3/widgets"
	ui "github.com/gizak/termui/v3"
)

type TextEntry struct {
	paragraph *widgets.Paragraph

	size ui.Resize

	action func (string) bool

	v View
}

func NewTextEntry(title string, action func (string) bool, v View) View {
	var size ui.Resize
	size.Width, size.Height = ui.TerminalDimensions()
	paragraph := widgets.NewParagraph()
	paragraph.Title = title
	paragraph.Text = "_"
	return &TextEntry {
		paragraph: paragraph,
		size: size,
		action: action,
		v: v,
	}
}

func (textEntry *TextEntry) Handle(e ui.Event) View {
	p := textEntry.paragraph
	switch e.ID {
	case "<Enter>":
	  if textEntry.action(p.Text[:len(p.Text)-1]) {
			return textEntry.v
		}
	case "<Escape>":
		return textEntry.v
	case "<Backspace>":
	  if len(p.Text) > 1 {
			p.Text = p.Text[:len(p.Text)-2] + "_"
		}
	case "<C-u>":
	  p.Text = "_"
	case "<Space>":
	  e.ID = " "
		fallthrough
	default:
	  p.Text = p.Text[:len(p.Text)-1] + e.ID + "_"
	}
	return textEntry
}

func (textEntry *TextEntry) Update() {
	width := 40
	height := 3
	midx := textEntry.size.Width / 2
	midy := textEntry.size.Height / 2
	x := midx - width / 2
	y := midy - height / 2
	textEntry.paragraph.SetRect(x, y, x + width, y + height)
}

func (textEntry TextEntry) Render() []ui.Drawable {
	return []ui.Drawable{textEntry.paragraph}
}
