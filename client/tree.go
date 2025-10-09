package client

import (
	"fmt"
	"image"
	"strings"

	ui "github.com/gizak/termui/v3"
	rw "github.com/mattn/go-runewidth"
)

const treeIndent = "  "

// TreeNode is a tree node.
type TreeNode struct {
	Value    fmt.Stringer
	Expanded bool
	Nodes    []*TreeNode

	// level stores the node level in the tree.
	level int
}

// TreeWalkFn is a function used for walking a Tree.
// To interrupt the walking process function should return <= 0
// To prevent steps down the tree, return <= 0
type TreeWalkFn func(*TreeNode) int

func (self *TreeNode) parseStyles(style ui.Style) []ui.Cell {
	var sb strings.Builder
	if len(self.Nodes) == 0 {
		sb.WriteString(strings.Repeat(treeIndent, self.level+1))
	} else {
		sb.WriteString(strings.Repeat(treeIndent, self.level))
		if self.Expanded {
			sb.WriteRune(ui.Theme.Tree.Expanded)
		} else {
			sb.WriteRune(ui.Theme.Tree.Collapsed)
		}
		sb.WriteByte(' ')
	}
	sb.WriteString(self.Value.String())
	return ui.ParseStyles(sb.String(), style)
}

// Tree is a tree widget.
type Tree struct {
	ui.Block
	TextStyle        ui.Style
	SelectedRowStyle ui.Style
	WrapText         bool
	SelectedRow      int
	Focused          bool

	nodes []*TreeNode
	// rows is flatten nodes for rendering.
	rows   []*TreeNode
	topRow int
}

// NewTree creates a new Tree widget.
func NewTree() *Tree {
	return &Tree{
		Block:            *ui.NewBlock(),
		TextStyle:        ui.Theme.Tree.Text,
		SelectedRowStyle: ui.Theme.Tree.Text,
		WrapText:         true,
	}
}

func (self *Tree) SetNodes(nodes []*TreeNode) {
	self.nodes = nodes
	self.prepareNodes()
}

func (self *Tree) prepareNodes() {
	self.rows = make([]*TreeNode, 0)
	for _, node := range self.nodes {
		self.prepareNode(node, 0)
	}
}

func (self *Tree) prepareNode(node *TreeNode, level int) {
	self.rows = append(self.rows, node)
	node.level = level

	if node.Expanded {
		for _, n := range node.Nodes {
			self.prepareNode(n, level+1)
		}
	}
}

func (self *Tree) Walk(fn TreeWalkFn) {
	for _, n := range self.nodes {
		if self.walk(n, fn) == 0 {
			break
		}
	}
}

func (self *Tree) walk(n *TreeNode, fn TreeWalkFn) int {
	v := fn(n)

	if v > 0 {
		for _, node := range n.Nodes {
			v = self.walk(node, fn)
			if v <= 0 {
				break
			}
		}
	}

	return v
}

func (self *Tree) Draw(buf *ui.Buffer) {
	self.Block.Draw(buf)
	point := self.Inner.Min

	// adjusts view into widget
	if self.SelectedRow >= self.Inner.Dy()+self.topRow {
		self.topRow = self.SelectedRow - self.Inner.Dy() + 1
	} else if self.SelectedRow < self.topRow {
		self.topRow = self.SelectedRow
	}

	// draw rows
	for row := self.topRow; row < len(self.rows) && point.Y < self.Inner.Max.Y; row++ {
		cells := self.rows[row].parseStyles(self.TextStyle)
		if self.WrapText {
			cells = ui.WrapCells(cells, uint(self.Inner.Dx()))
		}
		for j := 0; j < len(cells) && point.Y < self.Inner.Max.Y; j++ {
			style := cells[j].Style
			if self.Focused && row == self.SelectedRow {
				style = self.SelectedRowStyle
			}
			if point.X+1 == self.Inner.Max.X+1 && len(cells) > self.Inner.Dx() {
				buf.SetCell(ui.NewCell(ui.ELLIPSES, style), point.Add(image.Pt(-1, 0)))
			} else {
				buf.SetCell(ui.NewCell(cells[j].Rune, style), point)
				point = point.Add(image.Pt(rw.RuneWidth(cells[j].Rune), 0))
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
	if len(self.rows) > int(self.topRow)+self.Inner.Dy() {
		buf.SetCell(
			ui.NewCell(ui.DOWN_ARROW, ui.NewStyle(ui.ColorWhite)),
			image.Pt(self.Inner.Max.X-1, self.Inner.Max.Y-1),
		)
	}
}

// ScrollAmount scrolls by amount given. If amount is < 0, then scroll up.
// There is no need to set self.topRow, as this will be set automatically when drawn,
// since if the selected item is off screen then the topRow variable will change accordingly.
func (self *Tree) ScrollAmount(amount int) {
	if len(self.rows)-int(self.SelectedRow) <= amount {
		self.SelectedRow = len(self.rows) - 1
	} else if int(self.SelectedRow)+amount < 0 {
		self.SelectedRow = 0
	} else {
		self.SelectedRow += amount
	}
}

func (self *Tree) SelectedNode() *TreeNode {
	if len(self.rows) == 0 {
		return nil
	}
	return self.rows[self.SelectedRow]
}

func (self *Tree) ScrollUp() {
	self.ScrollAmount(-1)
}

func (self *Tree) ScrollDown() {
	self.ScrollAmount(1)
}

func (self *Tree) ScrollPageUp() {
	// If an item is selected below top row, then go to the top row.
	if self.SelectedRow > self.topRow {
		self.SelectedRow = self.topRow
	} else {
		self.ScrollAmount(-self.Inner.Dy())
	}
}

func (self *Tree) ScrollPageDown() {
	self.ScrollAmount(self.Inner.Dy())
}

func (self *Tree) ScrollHalfPageUp() {
	self.ScrollAmount(-int(ui.FloorFloat64(float64(self.Inner.Dy()) / 2)))
}

func (self *Tree) ScrollHalfPageDown() {
	self.ScrollAmount(int(ui.FloorFloat64(float64(self.Inner.Dy()) / 2)))
}

func (self *Tree) ScrollTop() {
	self.SelectedRow = 0
}

func (self *Tree) ScrollBottom() {
	self.SelectedRow = len(self.rows) - 1
}

func (self *Tree) Collapse() {
	self.rows[self.SelectedRow].Expanded = false
	self.prepareNodes()
}

func (self *Tree) Expand() {
	node := self.rows[self.SelectedRow]
	if len(node.Nodes) > 0 {
		self.rows[self.SelectedRow].Expanded = true
	}
	self.prepareNodes()
}

func (self *Tree) ToggleExpand() {
	node := self.rows[self.SelectedRow]
	if len(node.Nodes) > 0 {
		node.Expanded = !node.Expanded
	}
	self.prepareNodes()
}

func (self *Tree) ExpandAll() {
	self.Walk(func(n *TreeNode) int {
		if len(n.Nodes) > 0 {
			n.Expanded = true
		}
		return 1
	})
	self.prepareNodes()
}

func (self *Tree) CollapseAll() {
	self.Walk(func(n *TreeNode) int {
		n.Expanded = false
		return 1
	})
	self.prepareNodes()
}

func (self *Tree) Size() int {
	return len(self.rows)
}
