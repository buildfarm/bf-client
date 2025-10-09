package client

import (
	"image"

	ui "github.com/gizak/termui/v3"
)

type Paragraph struct {
	ui.Block
	Text      string
	TextStyle ui.Style
	WrapText  bool
	Raw       bool
}

func NewParagraph() *Paragraph {
	return &Paragraph{
		Block:     *ui.NewBlock(),
		TextStyle: ui.Theme.Paragraph.Text,
		WrapText:  true,
		Raw:       false,
	}
}

func rawCells(s string, defaultStyle ui.Style) []ui.Cell {
	cells := []ui.Cell{}
	runes := []rune(s)
	for _, _rune := range runes {
		cells = append(cells, ui.Cell{_rune, defaultStyle})
	}
	return cells
}

func (self *Paragraph) Draw(buf *ui.Buffer) {
	self.Block.Draw(buf)

	var cells []ui.Cell
	if self.Raw {
		cells = rawCells(self.Text, self.TextStyle)
	} else {
		cells = ui.ParseStyles(self.Text, self.TextStyle)
	}
	if self.WrapText {
		cells = ui.WrapCells(cells, uint(self.Inner.Dx()))
	}

	rows := ui.SplitCells(cells, '\n')

	for y, row := range rows {
		if y+self.Inner.Min.Y >= self.Inner.Max.Y {
			break
		}
		row = ui.TrimCells(row, self.Inner.Dx())
		for _, cx := range ui.BuildCellWithXArray(row) {
			x, cell := cx.X, cx.Cell
			buf.SetCell(cell, image.Pt(x, y).Add(self.Inner.Min))
		}
	}
}
