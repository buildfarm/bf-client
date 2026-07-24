package view

import (
	"strings"
	"github.com/gizak/termui/v3/widgets"
	ui "github.com/gizak/termui/v3"
)

type TextEntry struct {
	paragraph *widgets.Paragraph

	content string

	size ui.Resize

	action func (string) bool

	v View

	focus bool
}

func NewTextEntry(title string, action func (string) bool, size ui.Resize, v View) *TextEntry {
	paragraph := widgets.NewParagraph()
	paragraph.Title = title
	return &TextEntry {
		paragraph: paragraph,
		size: size,
		action: action,
		v: v,
		focus: true,
	}
}

func (textEntry *TextEntry) Handle(e ui.Event) View {
	switch e.ID {
	case "<Enter>":
	  if textEntry.action(textEntry.content) {
			return textEntry.v
		}
	case "<Escape>":
		return textEntry.v
	case "<Backspace>":
		if len(textEntry.content) > 0 {
			textEntry.content = textEntry.content[:len(textEntry.content)-1]
		}
	case "<C-u>":
	  textEntry.content = ""
	case "<Space>":
	  e.ID = " "
		fallthrough
	default:
	  textEntry.content += e.ID
	}
	return textEntry
}

func (textEntry *TextEntry) Update() {
	width := 40
	height := strings.Count(textEntry.content, "\n") + 3
	midx := textEntry.size.Width / 2
	midy := textEntry.size.Height / 2
	x := midx - width / 2
	y := midy - height / 2
	textEntry.paragraph.SetRect(x, y, x + width, y + height)
}

func (textEntry TextEntry) Render() []ui.Drawable {
	textEntry.paragraph.Text = textEntry.content
	if textEntry.focus {
		textEntry.paragraph.Text += "_"
	}
	return []ui.Drawable{textEntry.paragraph}
}
