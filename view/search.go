package view

import (
	"fmt"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/werkt/bf-client/client"
)

type dropdown struct {
	client.List
	values   []fmt.Stringer
	selected int
}

func newDropdown(values []fmt.Stringer) *dropdown {
	list := client.NewList()
	list.Rows = []fmt.Stringer{values[0]}
	list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	return &dropdown{
		List:     *list,
		values:   values,
		selected: 0,
	}
}

func (d *dropdown) open() {
	// if already closed
	if d.Size().Y == 3 {
		d.SetRect(d.Min.X, d.Min.Y, d.Max.X, d.Min.Y+len(d.values)+2)
		d.Rows = d.values
		d.SelectedRow = d.selected
	}
}

func (d *dropdown) close() {
	if d.SelectedRow != -1 {
		d.selected = d.SelectedRow
		d.SetRect(d.Min.X, d.Min.Y, d.Max.X, d.Min.Y+3)
		d.Rows = []fmt.Stringer{d.values[d.selected]}
		d.SelectedRow = -1
	}
}

func (d *dropdown) value() string {
	return d.values[d.selected].String()
}

type search struct {
	a        *client.App
	v        View
	i        int
	resource *dropdown
	filter   *dropdown
	text     *widgets.Paragraph
	layout   []ui.Drawable
	t        []ui.Drawable
}

type stringer struct {
	s string
}

func (s *stringer) String() string {
	return s.s
}

func makeStringers(values []string) []fmt.Stringer {
	stringers := make([]fmt.Stringer, len(values))
	for i, s := range values {
		stringers[i] = &stringer{s: s}
	}
	return stringers
}

func NewSearch(a *client.App, v View) View {
	resource := newDropdown(makeStringers([]string{"executions", "toolInvocations", "correlatedInvocations"}))
	resource.SetRect(20, 15, 40, 18)
	resource.Border = false
	filter := newDropdown(makeStringers([]string{"toolInvocationId", "correlatedInvocationsId", "username", "hostname", "name"}))
	filter.SetRect(resource.Max.X+1, resource.Min.Y, resource.Max.X+21, resource.Max.Y)
	filter.Border = false
	text := widgets.NewParagraph()
	text.Text = "_"
	text.SetRect(filter.Max.X+1, filter.Min.Y, filter.Max.X+40, filter.Max.Y)
	return &search{
		a:        a,
		v:        v,
		i:        0,
		resource: resource,
		filter:   filter,
		text:     text,
		t:        []ui.Drawable{text, resource, filter},
		layout:   []ui.Drawable{resource, filter, text},
	}
}

func (s *search) Update() {
}

func (s *search) handleDropdown(d *dropdown, e ui.Event) {
	switch e.ID {
	case "<Up>":
		d.open()
		d.ScrollUp()
	case "<Down>":
		d.open()
		d.ScrollDown()
	case "<Enter>":
		d.close()
	}
}

func (s *search) handleText(p *widgets.Paragraph, e ui.Event) {
	if e.ID == "<Backspace>" {
		if len(p.Text) > 1 {
			p.Text = p.Text[:len(p.Text)-2] + "_"
		}
	} else if e.ID == "<C-u>" {
		p.Text = "_"
	} else {
		p.Text = p.Text[:len(p.Text)-1] + e.ID + "_"
	}
}

func (s *search) Handle(e ui.Event) View {
	switch e.ID {
	case "<Tab>":
		switch s.i {
		case 0:
			s.text.Text = s.text.Text[:len(s.text.Text)-1]
			s.text.Border = false
		case 1:
			s.resource.close()
			s.resource.Border = false
		case 2:
			s.filter.close()
			s.filter.Border = false
		}
		s.i = (s.i + 1) % len(s.t)
		switch s.i {
		case 0:
			s.text.Border = true
			s.text.Text = s.text.Text + "_"
		case 1:
			s.resource.Border = true
		case 2:
			s.filter.Border = true
		}
		return s
	case "<Escape>":
		return s.v
	case "<Enter>":
		if s.i == 0 && len(s.text.Text) != 1 {
			text := s.text.Text[:len(s.text.Text)-1]
			if s.resource.value() == "executions" {
				if s.filter.value() == "name" {
					return NewDocument(s.a, s.a.Instance+"/executions/"+text, s.v)
				}
				olv := NewOperationList(s.a, 4, s.v)
				olv.Filter = fmt.Sprintf("%s=%s", s.filter.value(), text)
				return olv
			}
			return NewSearchResults(s.resource.value(), s.filter.value(), text, s.a, s.v)
		}
	case "q":
		if s.i != 0 {
			return s.v
		}
	}

	switch s.i {
	case 0: // text input
		s.handleText(s.text, e)
	case 1: // resource dropdown
		s.handleDropdown(s.resource, e)
	case 2: // filter dropdown
		s.handleDropdown(s.filter, e)
	}

	return s
}

func (v search) Render() []ui.Drawable {
	return v.layout
}
