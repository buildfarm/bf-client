package view

import (
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/golang/protobuf/proto"
	"github.com/werkt/bf-client/client"
)

type commandView struct {
	a       *client.App
	d       bfpb.Digest
	command *reapi.Command
	err     error
	v       View
}

func NewCommand(a *client.App, d bfpb.Digest, v View) View {
	return &commandView{
		a: a,
		d: d,
		v: v,
	}
}

func (v *commandView) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return v.v
	}
	return v
}

func (v *commandView) Update() {
	c := &reapi.Command{}
	if err := client.Expect(v.a.Conn, v.d, c); err != nil {
		v.err = err
	} else {
		v.command = c
	}
}

func (v commandView) Render() []ui.Drawable {
	p := widgets.NewParagraph()
	p.Title = client.DigestString(v.d)
	p.WrapText = true
	p.SetRect(0, 0, 120, 60)
	if v.err != nil {
		p.Text = string(v.err.Error())
	} else {
		p.Text = proto.MarshalTextString(v.command)
	}
	return []ui.Drawable{p}
}
