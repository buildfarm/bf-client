package view

import (
	"strings"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/buildfarm/bf-client/client"
	"golang.org/x/net/html"
)

type edit struct {
	digest bfpb.Digest
	action reapi.Action
	command reapi.Command
	input reapi.Directory
	index map[string]reapi.Directory
	p *widgets.Paragraph
	c client.Component
	v View

	source bool
	d *client.Document
	cn *html.Node
	in *html.Node
	anchors []*html.Node
	focusAnchor int
}

func NewEdit(name string, c client.Component, v View) *edit {
	d := client.NewDocument()
	content := `
<html>
  <head>
    <title></title>
  </head>
  <body>
    <h2>Command</h2>
    <ul id="command"></ul>
    <h2>Input</h2>
    <ul id="input"></ul>
  </body>
</html>`
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	d.SetRoot(root)
	if len(name) == 0 {
		name = "<Untitled>"
	}
	client.DocumentSetText(d.Find("title"), name)
	command := d.Find("#command")
	input := d.Find("#input")

	e := &edit {
		p: widgets.NewParagraph(),
		c: c,
		v: v,

		d: d,
		cn: command,
		in: input,
	}

	e.renderCommand(node { node: command })
	e.renderInput(node { node: input })
	d.Update()

	e.anchors = d.FindAll("a")
	if len(e.anchors) > 0 {
		focus(e.anchors[e.focusAnchor])
	}

	size := c.Dimensions()
	e.p.SetRect(0, 0, size.Width, size.Height)
	return e
}

func (e *edit) renderCommand(n node) {
	a := li().text("Arguments:")
	ol := ol().id("arguments")
	for _, arg := range e.command.Arguments {
		ol.appendNode(li().text(arg))
	}
	a.appendNode(ol)
	a.appendNode(frag(`<a href="arguments:add">Add</a>`))
	n.appendNode(a)
}

func (e *edit) renderDirectory(n node, d reapi.Directory) {
	ul := ul()
	for _, d := range d.Directories {
		l := li().text(d.Name)
		if d.Digest.SizeBytes != 0 {
			e.renderDirectory(l, e.index[digestString(*d.Digest)])
		}
		ul.appendNode(l)
	}
	for _, f := range d.Files {
		ul.appendNode(li().text(f.Name))
	}
	n.appendNode(ul)
}

func (e *edit) renderInput(n node) {
	i := li().text("/")
	e.renderDirectory(i, e.input)
	// need to make this follow the active directory selection
	i.appendNode(frag(`<a href="add:input">Add</a>`))
	n.appendNode(i)
}

func (e *edit) Update() {
}

func (e edit) Render() []ui.Drawable {
	e.p.Title = e.d.Title()
	if e.source {
		e.p.Text = e.d.RenderSource()
	} else {
		e.p.Text = e.d.Render()
	}
	return []ui.Drawable { e.p }
}

func (e *edit) changeName(name string) bool {
	e.p.Title = name
	return true
}

func (e *edit) addArgument(argument string) bool {
	/* probably connect model... */
	e.command.Arguments = append(e.command.Arguments, argument)
	e.cn.FirstChild, e.cn.LastChild = nil, nil
	e.renderCommand(node { node: e.cn })
	e.d.Update()

	e.anchors = e.d.FindAll("a")
	if len(e.anchors) > 0 {
		focus(e.anchors[e.focusAnchor])
	}
	return true
}

func (e *edit) link(target string, inner string) View {
	tc := strings.SplitN(target, ":", 2)
	view, id := tc[0], tc[1]
	if view == "arguments" {
		if id == "add" {
			return NewTextEntry("Add Argument", e.addArgument, e.c.Dimensions(), e)
		}
	}
	return link(e.c, target, inner, e)
}

func (ed *edit) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return ed.v
	case "r":
		return NewTextEntry("Change Action Name", ed.changeName, ed.c.Dimensions(), ed)
	case "u":
		ed.source = !ed.source
	case "R":
		return NewRun(ed.digest, ed.c, ed)
	/* awful lot of repetition here */
	case "<Tab>", "j", "<Down>":
		prevAnchor := ed.anchors[ed.focusAnchor]
		ed.focusAnchor = (ed.focusAnchor + 1) % len(ed.anchors)
		defocus(prevAnchor)
		focus(ed.anchors[ed.focusAnchor])
	case "k", "<Up>":
		prevAnchor := ed.anchors[ed.focusAnchor]
		anchors := len(ed.anchors)
		ed.focusAnchor = (ed.focusAnchor + anchors - 1) % anchors
		defocus(prevAnchor)
		focus(ed.anchors[ed.focusAnchor])
	case "<Enter>":
		anchor := ed.anchors[ed.focusAnchor]
		href, err := getAttr(anchor, "href")
		if err == nil {
			name, _ := getAttr(anchor, "name")
			return ed.link(href, name)
		}
	}
	return ed
}
