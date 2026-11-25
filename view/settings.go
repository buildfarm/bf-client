package view

import (
	"strconv"
	"time"

	"github.com/buildfarm/bf-client/client"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

type settings struct {
	v        View
	fpsEntry *entry
	upsEntry *entry
	layout   []ui.Drawable
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
	fpsLabel := widgets.NewParagraph()
	fpsLabel.SetRect(10, 15, 25, 18)
	fpsLabel.Border = false
	fpsLabel.Text = "Frames/sec"

	fpsEntry := newEntry()
	fpsEntry.SetRect(25, 15, 47, 18)
	fpsEntry.Text = strconv.Itoa(a.FramesPerSecond)
	fpsEntry.focus()

	upsLabel := widgets.NewParagraph()
	upsLabel.SetRect(10, 20, 25, 23)
	upsLabel.Border = false
	upsLabel.Text = "Updates/sec"

	upsText := func() string {
		return strconv.FormatFloat(a.UpdatesPerSecond, 'g', -1, 64)
	}
	upsEntry := newEntry()
	upsEntry.SetRect(25, 20, 52, 23)
	upsEntry.Text = upsText()

	fpsEntry.onEnter = func(s string) string {
		if fps, err := strconv.Atoi(s); err == nil && fps > 0 && fps <= int(time.Second) {
			a.SetRates(fps, a.UpdatesPerSecond)
			upsEntry.Text = upsText()
		}
		return strconv.Itoa(a.FramesPerSecond)
	}
	upsEntry.onEnter = func(s string) string {
		if ups, err := strconv.ParseFloat(s, 64); err == nil {
			if ups == 0 {
				ups = 1
			}
			if client.ValidUpdatesPerSecond(ups) {
				a.SetRates(a.FramesPerSecond, ups)
			}
		}
		return upsText()
	}

	return &settings{
		v:        v,
		fpsEntry: fpsEntry,
		upsEntry: upsEntry,
		layout:   []ui.Drawable{fpsLabel, upsLabel, fpsEntry, upsEntry},
	}
}

func (s *settings) Update() {
}

func (s *settings) Handle(e ui.Event) View {
	switch e.ID {
	case "<Tab>":
		if s.fpsEntry.focused {
			s.fpsEntry.defocus()
			s.upsEntry.focus()
		} else {
			s.upsEntry.defocus()
			s.fpsEntry.focus()
		}
		return s
	case "<Escape>":
		return s.v
	}

	if s.fpsEntry.focused {
		s.fpsEntry.handle(e)
	} else if s.upsEntry.focused {
		s.upsEntry.handle(e)
	}

	return s
}

func (v settings) Render() []ui.Drawable {
	return v.layout
}
