package view

import (
	"fmt"
	"strings"

	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/werkt/bf-client/client"
	"golang.org/x/net/html"
)

type actionView struct {
	a          *client.App
	d          bfpb.Digest
	action     *reapi.Action
	err        error
	v          View
	doc        *client.Document
	p          *widgets.Paragraph
	actionNode *html.Node
	source     bool

	anchors     []*html.Node
	focusAnchor int
}

func NewAction(a *client.App, d bfpb.Digest, v View) View {
	doc := client.NewDocument()
	content := `
  <html>
    <head>
      <title></title>
    </head>
    <body>
      <div id="action"></div>
    </body>
  </html>`
	root, err := html.Parse(strings.NewReader(content))
	doc.SetRoot(root)
	client.DocumentSetText(doc.Find("title"), client.DigestString(d))
	actionNode := doc.Find("#action")
	if err != nil {
		panic(err)
	}
	doc.Update()

	focusAnchor := 0
	anchors := doc.FindAll("a")
	if len(anchors) > 0 {
		focus(anchors[focusAnchor])
	}

	return &actionView{
		a:           a,
		d:           d,
		v:           v,
		p:           widgets.NewParagraph(),
		actionNode:  actionNode,
		doc:         doc,
		anchors:     anchors,
		focusAnchor: focusAnchor,
	}
}

// this needs global registration
func (v *actionView) link(target string) View {
	c := strings.SplitN(target, ":", 2)
	view, id := c[0], c[1]
	switch view {
	case "command":
		return NewCommand(v.a, client.ParseDigest(id), v)
	case "input":
		return NewInput(v.a, client.ParseDigest(id), v)
	}
	return v
}
func (v *actionView) Handle(e ui.Event) View {
	switch e.ID {
	case "<Tab>", "j", "<Down>":
		prevAnchor := v.anchors[v.focusAnchor]
		v.focusAnchor = (v.focusAnchor + 1) % len(v.anchors)
		defocus(prevAnchor)
		focus(v.anchors[v.focusAnchor])
		return v
	case "k", "<Up>":
		prevAnchor := v.anchors[v.focusAnchor]
		anchors := len(v.anchors)
		v.focusAnchor = (v.focusAnchor + anchors - 1) % anchors
		defocus(prevAnchor)
		focus(v.anchors[v.focusAnchor])
		return v
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return v.v
	case "<Enter>":
		anchor := v.anchors[v.focusAnchor]
		href, err := getAttr(anchor, "href")
		if err == nil {
			return v.link(href)
		}
		return v
	}
	return v
}

func (v *actionView) Update() {
	if v.action == nil {
		a := &reapi.Action{}
		err := client.Expect(v.a.Conn, v.d, a)
		if err != nil {
			v.err = err
		} else {
			v.action = a
		}
		content := `
    <div>Command: <a href="command:%[1]s">%[1]s</a></div>
    <div>Input Root: <a href="input:%[2]s">%[2]s</a></div>`
		command := renderREDigest(*a.CommandDigest, v.d.DigestFunction, false)
		inputRoot := renderREDigest(*a.InputRootDigest, v.d.DigestFunction, false)
		content = fmt.Sprintf(content, command, inputRoot)
		if len(a.Platform.Properties) > 0 {
			content += "<h2>Platform:</h2><ul>"
			for _, property := range a.Platform.Properties {
				content += fmt.Sprintf("<li>%s: %s</li>", property.Name, property.Value)
			}
			content += "</ul>"
		}
		replaceNodeContent(content, v.actionNode)

		anchors := v.doc.FindAll("a")
		if len(v.anchors) > 0 {
			a := v.anchors[v.focusAnchor]
			v.focusAnchor = -1
			for i, da := range anchors {
				// crude
				if href(da) == href(a) {
					v.focusAnchor = i
				}
			}
			if v.focusAnchor == -1 {
				// maybe figure out how to jump back to our link...
				v.focusAnchor = 0
			}
		}
		v.anchors = anchors
		focus(v.anchors[v.focusAnchor])
		v.doc.Update()
	}
}

func (v actionView) Render() []ui.Drawable {
	v.p.Title = v.doc.Title()
	if v.source {
		v.p.Text = v.doc.RenderSource()
	} else {
		v.p.Text = v.doc.Render()
	}
	v.p.SetRect(0, 0, 120, 60)
	/*
	  if v.err != nil {
	    p.Text = string(v.err.Error())
	  }
	*/
	return []ui.Drawable{v.p}
}
