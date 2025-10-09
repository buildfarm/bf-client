package view

import (
	"fmt"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/werkt/bf-client/client"
	"strconv"
)

type settings struct {
	a               *client.App
	v               View
	limitEntry      *entry
	skipFramesEntry *entry
	layout          []ui.Drawable
}

type entry struct {
	widgets.Paragraph
	focused bool
	onEnter func(string) string
}

func newEntry() *entry {
	p := widgets.NewParagraph()
	return &entry{
		Paragraph: *p,
	}
}

func (e *entry) focus() {
	if !e.focused {
		e.Text += "_"
		e.focused = true
	}
}

func (e *entry) defocus() {
	if e.focused {
		e.Text = e.Text[:len(e.Text)-1]
		e.focused = false
	}
	e.Text = e.onEnter(e.Text)
}

func (e *entry) handle(ue ui.Event) {
	if ue.ID == "<Enter>" {
		e.defocus()
	} else {
		if ue.ID == "<Backspace>" {
			if len(e.Text) > 1 {
				e.Text = e.Text[:len(e.Text)-2] + "_"
			}
		} else if ue.ID == "<C-u>" {
			e.Text = "_"
		} else {
			e.Text = e.Text[:len(e.Text)-1] + ue.ID + "_"
		}
	}
}

func newSettings(a *client.App, v View) View {
	limit := widgets.NewParagraph()
	limit.SetRect(10, 15, 20, 18)
	limit.Border = false
	limit.Text = "Frame Limit"

	limitEntry := newEntry()
	limitEntry.SetRect(22, 15, 32, 18)
	limitEntry.Text = fmt.Sprintf("%d", a.FrameLimit)
	limitEntry.focus()

	skip := widgets.NewParagraph()
	skip.SetRect(10, 20, 20, 23)
	skip.Border = false
	skip.Text = "Update Frame Skip"

	skipEntry := newEntry()
	skipEntry.Text = fmt.Sprintf("%d", a.SkipFrames)
	skipEntry.SetRect(22, 20, 32, 23)

	settings := &settings{
		a:               a,
		v:               v,
		limitEntry:      limitEntry,
		skipFramesEntry: skipEntry,
		layout:          []ui.Drawable{limit, skip, limitEntry, skipEntry},
	}

	limitEntry.onEnter = func(s string) string {
		settings.a.FrameLimit, _ = strconv.Atoi(s)
		return fmt.Sprintf("%d", settings.a.FrameLimit)
	}
	skipEntry.onEnter = func(s string) string {
		settings.a.SkipFrames, _ = strconv.Atoi(s)
		settings.a.UpdateCountdown = 0
		return fmt.Sprintf("%d", settings.a.SkipFrames)
	}

	return settings
}

func (s *settings) Update() {
}

func (s *settings) Handle(e ui.Event) View {
	switch e.ID {
	case "<Tab>":
		if s.limitEntry.focused {
			s.limitEntry.defocus()
			s.skipFramesEntry.focus()
		} else {
			s.skipFramesEntry.defocus()
			s.limitEntry.focus()
		}
		return s
	case "<Escape>":
		return s.v
	}

	if s.limitEntry.focused {
		s.limitEntry.handle(e)
	} else if s.skipFramesEntry.focused {
		s.skipFramesEntry.handle(e)
	}

	return s
}

func (v settings) Render() []ui.Drawable {
	return v.layout
}
