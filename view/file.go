package view

import (
	"fmt"
	"strings"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/werkt/bf-client/client"
)

type file struct {
	a *client.App
	p          *widgets.Paragraph
	d bfpb.Digest
	fetched bool
	v View
	err error
	lines []string
	start int
}

func NewFile(a *client.App, d bfpb.Digest, s string, v View) View {
	p := widgets.NewParagraph()
	p.Title = ""
	if len(s) != 0 {
		p.Title = s + " "
	}
	p.Title += client.DigestString(d)
	p.WrapText = false
	p.SetRect(0, 0, 120, 60)
	return &file {
		a: a,
		p: p,
		d: d,
		fetched: false,
		v: v,
	}
}

func (f file) Render() []ui.Drawable {
	return []ui.Drawable{f.p}
}

func (f *file) Update() {
	if f.err == nil && !f.fetched {
		f.err = f.download()
	}
}

func trimToView(start int, height int, lines[] string) string {
	return strings.Join(lines[start:start + height], "\n")
}

func (f *file) download() error {
	b, err := client.ReadBlob(f.a.Conn, f.a.Instance, f.d)
	if err != nil {
		return err
	}
	f.lines = strings.Split(string(b), "\n")
	f.p.Title += fmt.Sprintf(" %d", len(f.lines))
	f.p.Text = trimToView(0, Min(58, len(f.lines)), f.lines)
	f.fetched = true
	return nil
}

func (f *file) offset(delta int) {
	f.start += delta

	l := len(f.lines)
	if f.start + 58 > l {
		f.start = l - 58
	}
	if f.start < 0 {
		f.start = 0
	}
	f.p.Text = trimToView(f.start, Min(58, len(f.lines)), f.lines)
}

func (f *file) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return f.v
	case "<PageDown>":
		f.offset(58)
	case "<PageUp>":
		f.offset(-58)
	default:
		panic(e.ID)
	}
	return f
}
