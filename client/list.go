// Copyright 2017 Zack Guo <zack.y.guo@gmail.com>. All rights reserved.
// Use of this source code is governed by a MIT license that can
// be found in the LICENSE file.

package client

import (
	"fmt"
	"image"

	rw "github.com/mattn/go-runewidth"

	ui "github.com/gizak/termui/v3"
)

type List struct {
	ui.Block
	Rows             []fmt.Stringer
	WrapText         bool
	TextStyle        ui.Style
	SelectedRow      int
	topRow           int
	SelectedRowStyle ui.Style
	SubTitle         fmt.Stringer
	SubTitleStyle    ui.Style
}

type emptyStringer struct {
}

func (e emptyStringer) String() string {
	return ""
}

func NewList() *List {
	return &List{
		Block:            *ui.NewBlock(),
		TextStyle:        ui.Theme.List.Text,
		SelectedRowStyle: ui.Theme.List.Text,
		SubTitle:         &emptyStringer{},
		SubTitleStyle:    ui.Theme.List.Text,
	}
}

func (self *List) Draw(buf *ui.Buffer) {
	self.Block.Draw(buf)

	subTitle := self.SubTitle.String()
	subTitleLen := len(subTitle)
	if subTitleLen > 0 {
		buf.SetString(
			subTitle,
			self.SubTitleStyle,
			image.Pt(self.Max.X-2-subTitleLen, self.Min.Y),
		)
	}

	point := self.Inner.Min

	// adjusts view into widget
	if self.SelectedRow >= self.Inner.Dy()+self.topRow {
		self.topRow = self.SelectedRow - self.Inner.Dy() + 1
	} else if self.SelectedRow < self.topRow {
		self.topRow = self.SelectedRow
	}
	if self.topRow < 0 {
		self.topRow = 0
	}

	// draw rows
	for row := self.topRow; row < len(self.Rows) && point.Y < self.Inner.Max.Y; row++ {
		cells := ui.ParseStyles(self.Rows[row].String(), self.TextStyle)
		if self.WrapText {
			cells = ui.WrapCells(cells, uint(self.Inner.Dx()))
		}
		for j := 0; j < len(cells) && point.Y < self.Inner.Max.Y; j++ {
			style := cells[j].Style
			if row == self.SelectedRow {
				style = self.SelectedRowStyle
			}
			if cells[j].Rune == '\n' {
				point = image.Pt(self.Inner.Min.X, point.Y+1)
			} else {
				if point.X+1 == self.Inner.Max.X+1 && len(cells) > self.Inner.Dx() {
					buf.SetCell(ui.NewCell(ui.ELLIPSES, style), point.Add(image.Pt(-1, 0)))
					break
				} else {
					buf.SetCell(ui.NewCell(cells[j].Rune, style), point)
					point = point.Add(image.Pt(rw.RuneWidth(cells[j].Rune), 0))
				}
			}
		}
		point = image.Pt(self.Inner.Min.X, point.Y+1)
	}

	// draw UP_ARROW if needed
	if self.topRow > 0 {
		buf.SetCell(
			ui.NewCell(ui.UP_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(self.Inner.Max.X-1, self.Inner.Min.Y),
		)
	}

	// draw DOWN_ARROW if needed
	if len(self.Rows) > int(self.topRow)+self.Inner.Dy() {
		buf.SetCell(
			ui.NewCell(ui.DOWN_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(self.Inner.Max.X-1, self.Inner.Max.Y-1),
		)
	}
}

// ScrollAmount scrolls by amount given. If amount is < 0, then scroll up.
// There is no need to set self.topRow, as this will be set automatically when drawn,
// since if the selected item is off screen then the topRow variable will change accordingly.
func (self *List) ScrollAmount(amount int) {
	if len(self.Rows)-int(self.SelectedRow) <= amount {
		self.SelectedRow = len(self.Rows) - 1
	} else if int(self.SelectedRow)+amount < 0 {
		self.SelectedRow = 0
	} else {
		self.SelectedRow += amount
	}
}

func (self *List) ScrollUp() {
	self.ScrollAmount(-1)
}

func (self *List) ScrollDown() {
	self.ScrollAmount(1)
}

func (self *List) ScrollPageUp() {
	// If an item is selected below top row, then go to the top row.
	if self.SelectedRow > self.topRow {
		self.SelectedRow = self.topRow
	} else {
		self.ScrollAmount(-self.Inner.Dy())
	}
}

func (self *List) ScrollPageDown() {
	self.ScrollAmount(self.Inner.Dy())
}

func (self *List) ScrollHalfPageUp() {
	self.ScrollAmount(-int(ui.FloorFloat64(float64(self.Inner.Dy()) / 2)))
}

func (self *List) ScrollHalfPageDown() {
	self.ScrollAmount(int(ui.FloorFloat64(float64(self.Inner.Dy()) / 2)))
}

func (self *List) ScrollTop() {
	self.SelectedRow = 0
}

func (self *List) ScrollBottom() {
	self.SelectedRow = len(self.Rows) - 1
}
