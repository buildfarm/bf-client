package view

import (
	"os"
	"fmt"
	"slices"
	ui "github.com/gizak/termui/v3"
	"github.com/buildfarm/bf-client/client"
)

type diffMenu struct {
	path string

	modal int

	menu *client.List

	c client.Component

	v View

	nextView View
}

type modal interface {
	render() []ui.Drawable
	init()
	update()
	handle(e ui.Event)
}

type action struct {
	name string
	m modal
}

func (a action) String() string {
	return a.name
}

type newDiff struct {
	i0 *TextEntry
	i1 *TextEntry

	d *diffMenu
}

func (n *newDiff) nextEntry(s string) bool {
	n.i0.focus, n.i1.focus = n.i1.focus, n.i0.focus
	return false
}

func (n *newDiff) createOrNext(s string) bool {
	// check to see that both entries work
	complete := len(n.i0.content) > 0 && len(n.i1.content) > 0

	if complete {
		// view or next view...
		n.d.nextView = CreateDiff(n.d.c, n.d.path, n.i0.content, n.i1.content, n.d.v)
	} else {
		n.i0.focus, n.i1.focus = n.i1.focus, n.i0.focus
	}
	return complete
}

func (n *newDiff) init() {
	// two entries
	n.i0 = NewTextEntry("Invocation A", n.nextEntry, UISize(), nil)
	n.i1 = NewTextEntry("Invocation B", n.createOrNext, UISize(), nil)
	// reposition to new middle
	n.i0.size.Height -= 6
	n.i1.size.Height += 6
	n.i1.focus = false
}

func (n *newDiff) update() {
	n.i0.Update()
	n.i1.Update()
}

func (n *newDiff) handle(e ui.Event) {
	if n.i0.focus {
		n.i0.Handle(e)
	} else {
		n.i1.Handle(e)
	}
}

func (n newDiff) render() []ui.Drawable {
	return slices.Concat(n.i0.Render(), n.i1.Render())
}

type diffEntry struct {
	name string
}

func (e diffEntry) String() string {
	return e.name
}

type loadDiff struct {
	l *client.List

	d *diffMenu
	action func(string) View
}

func (l *loadDiff) init() {
	c, err := os.ReadDir(l.d.path)
	if err != nil {
		panic(err)
	}
	l.l.Rows = make([]fmt.Stringer, len(c))
	for i, entry := range c {
		l.l.Rows[i] = &diffEntry{name: entry.Name()}
	}
	l.l.SetRect(10, 10, 120, 10 + len(l.l.Rows) + 2)
	l.l.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
}

func (l *loadDiff) update() {
}

func (d *diffMenu) load(name string) View {
	return LoadDiff(d.c, d.path, name, d.v)
}

// need to update the list after we delete as well
func (d *diffMenu) delete(name string) View {
	// return Confirm(fmt.Sprintf("Delete %s?"), v, func () View { deleteDiff(d, name); return v })
	os.RemoveAll(d.path + "/" + name)
	return d
}

func (l *loadDiff) handle(e ui.Event) {
	switch (e.ID) {
	case "j", "<Down>":
		l.l.ScrollDown()
	case "k", "<Up>":
		l.l.ScrollUp()
	case "<Enter>":
		l.d.nextView = l.action(l.l.Rows[l.l.SelectedRow].(*diffEntry).name)
	}
}

func (l loadDiff) render() []ui.Drawable {
	return []ui.Drawable { l.l }
}

func NewDiffMenu(c client.Component, path string, v View) View {
	// main menu
	// new -> ids
	// load -> select
	// delete -> select -> confirm
	// path -> modify
	menu := client.NewList()
	menu.SetRect(10, 10, 40, 15)
	menu.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	d := &diffMenu {
		path: path,
		modal: -1,
		menu: menu,
		c: c,
		v: v,
	}
	d.nextView = d
	menu.Rows = []fmt.Stringer {
		&action { name: "New", m: &newDiff{ d: d } },
		&action { name: "Load", m: &loadDiff{ d: d, l: client.NewList(), action: d.load } },
		&action { name: "Delete", m: &loadDiff{ d: d, l: client.NewList(), action: d.delete } },
		// &action { name: "Path", m: &changePath{ d: d } },
	}
	return d
}

func (d diffMenu) Render() []ui.Drawable {
	visible := []ui.Drawable { d.menu }
	if d.modal != -1 {
		for _, drawable := range d.menu.Rows[d.menu.SelectedRow].(*action).m.render() {
			visible = append(visible, drawable)
		}
	}
	return visible
}

func (d *diffMenu) Update() {
	if d.modal != -1 {
		d.menu.Rows[d.modal].(*action).m.update()
	}
}

func (d *diffMenu) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "<C-c>":
		return d.v
	case "q":
		if d.modal == -1 {
			return d.v
		}
	case "j", "<Down>":
		d.menu.ScrollDown()
	case "k", "<Up>":
		d.menu.ScrollUp()
	case "<Enter>":
		if d.modal == -1 {
			d.modal = d.menu.SelectedRow
			d.menu.Rows[d.modal].(*action).m.init()
			return d
		}
	}
	if d.modal >= 0 && d.modal < len(d.menu.Rows) {
		d.menu.Rows[d.modal].(*action).m.handle(e)
	}
	return d.nextView
}
