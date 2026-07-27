package view

import (
	"fmt"
	"strings"
	ui "github.com/gizak/termui/v3"
	"github.com/buildfarm/bf-client/client"
	"google.golang.org/genproto/googleapis/longrunning"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"golang.org/x/net/html"
)

type opDiff struct {
	p *client.Paragraph
	doc *client.Document
	tree *html.Node
	ops [2]*longrunning.Operation
	d *diff
	fetched bool
	err error
	i map[string]*reapi.Directory
	df reapi.DigestFunction_Value
	c client.Component
	source bool
	anchors     []*html.Node
	focusAnchor int
}

func NewOperationDiff(c client.Component, d *diff, ops [2]*longrunning.Operation, df reapi.DigestFunction_Value) View {
	doc := client.NewDocument()
	content := `
<html>
  <head>
    <title></title>
  </head>
  <body>
    <h2><a href="execution:%[1]s">%[1]s</a> - <a href="execution:%[2]s">%[2]s</a></h2>
    <ul id="tree"></ul>
  </body>
</html>
`
	root, err := html.Parse(strings.NewReader(fmt.Sprintf(content, ops[0].Name, ops[1].Name)))
	if err != nil {
		panic(err)
	}
	doc.SetRoot(root)
	m0 := client.RequestMetadata(ops[0])
	title := fmt.Sprintf("%s %s", m0.ActionMnemonic, m0.TargetId)
	client.DocumentSetText(doc.Find("title"), title)
	tree := doc.Find("#tree")
	doc.Update()
	return &opDiff {
		c: c,
		p: client.NewParagraph(),
		doc: doc,
		tree: tree,
		ops: ops,
		d: d,
		df: df,
	}
}

func (od opDiff) Render() []ui.Drawable {
	ui.Clear()
	od.p.Title = od.doc.Title()
	if od.source {
		od.p.Text = od.doc.RenderSource()
	} else {
		od.p.Text = od.doc.Render()
	}
	dimensions := od.c.Dimensions()
	od.p.SetRect(0, 0, dimensions.Width, dimensions.Height)
	return []ui.Drawable{od.p}
}

func (od *opDiff) Update() {
	if od.err == nil && !od.fetched {
		od.download()
		od.fetched = true
		od.doc.Update()
		anchors := od.doc.FindAll("a")
		// need to commonize
		if len(od.anchors) > 0 {
			a := od.anchors[od.focusAnchor]
			od.focusAnchor = -1
			for i, da := range anchors {
				// crude
				if href(da) == href(a) {
					od.focusAnchor = i
				}
			}
			if od.focusAnchor == -1 {
				// maybe figure out how to jump back to our link...
				od.focusAnchor = 0
			}
		}
		od.anchors = anchors
		focus(od.anchors[od.focusAnchor])
	}
}

func (od *opDiff) dirKey(d *reapi.Digest) string {
	return client.DigestString(client.ToDigest(*d, od.df))
}

func (od *opDiff) dir(d *reapi.Digest) *reapi.Directory {
	return od.i[od.dirKey(d)]
}

type pair struct {
	p string
	d [2]*reapi.Digest
	el node
}

func (p pair) path(name string) string {
	if p.p == "" {
		return name
	}
	return p.p + "/" + name
}

func (od *opDiff) download() {
	a0, a1 := od.d.actions[0][getActionDigest(od.ops[0])], od.d.actions[1][getActionDigest(od.ops[1])]
	d0, d1 := a0.InputRootDigest, a1.InputRootDigest

	od.i = make(map[string]*reapi.Directory)
	client.FetchTree(client.ToDigest(*d0, od.df), od.i, od.c.App().Conn)
	client.FetchTree(client.ToDigest(*d1, od.df), od.i, od.c.App().Conn)

	// must be visible since we have something

	dirs := []pair{ pair{ el: node{node: od.tree}, p: "", d: [2]*reapi.Digest{ d0, d1 } } }

	total := len(dirs)
	completed := 0
	for len(dirs) > 0 {
		var p pair
		p, dirs = dirs[0], dirs[1:]
		d0, d1 := od.dir(p.d[0]), od.dir(p.d[1])

		for i, fn0 := range d0.Files {
			fn1 := d1.Files[i]
			if fn0.Name != fn1.Name {
				panic("file names do not match")
			}
			s0, s1 := client.DigestString(client.ToDigest(*fn0.Digest, od.df)), client.DigestString(client.ToDigest(*fn1.Digest, od.df))
			text := "file"
			if s0 != s1 {
				text += " " + fn0.Name
				fel := li().text(fn0.Name).frag(fmt.Sprintf(`<a href="file:%[1]s">%[1]s</a> - <a href="file:%[2]s">%[2]s</a>`, s0, s1))
				p.el.appendNode(fel)
				// Produced by op0 - op1
				fk0, fk1 := digestString(*fn0.Digest), digestString(*fn1.Digest)
				ops0, ops1 := od.d.digests[0][fk0], od.d.digests[1][fk1]
				// could have asymmetries
				if len(ops0) != len(ops1) {
					panic("different number of ops produced digests")
				}
				// 1. ignore the current operations themselves
				// 2. ignore anything that has an input translation... so many do
				// 3. ... prefer something with the same configurationId??
				// we need to link to the view of the paired operation itself
				for i, j := 0, 0; i < len(ops0) && j < len(ops1); {
					if ops0[i] == od.ops[0].Name {
						i++
					}
					if ops1[j] == od.ops[1].Name {
						j++
					}
					if i < len(ops0) && j < len(ops1) {
						el := div().frag(fmt.Sprintf(`Produced by <a href="execution:%[1]s">%[1]s</a> and <a href="execution:%[2]s">%[2]s</a>`, ops0[i], ops1[j]))
						fel.appendNode(el)
					}
					i++
					j++
				}
				// show up the tree
				n := p.el.node
				route := text
				for n != od.tree {
					route += " < " + n.Data
					// n is ul, Parent is li, Parent.Parent is ul or null
					client.Show(n)
					if n.Parent == od.tree {
						panic(n.Data + " is incorrectly routed: " + route)
					}
					route += " : " + n.Parent.Data
					n = n.Parent.Parent
				}
				// compute diff
			}
		}

		for i, dn0 := range d0.Directories {
			dn1 := d1.Directories[i]
			if dn0.Name != dn1.Name {
				panic("directory names do not match")
			}
			if digestString(*dn0.Digest) != digestString(*dn1.Digest) {
				el := ul()
				p.el.appendNode(li().text(dn0.Name).appendNode(el))
				client.Hide(el.node)
				dirs = append(dirs, pair{ el: el, p: p.path(dn0.Name), d: [2]*reapi.Digest{ dn0.Digest, dn1.Digest } })
				total++
			}
		}
		completed++
	}
}

func (od *opDiff) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return od.d
	case "<Tab>", "j", "<Down>":
		prevAnchor := od.anchors[od.focusAnchor]
		od.focusAnchor = (od.focusAnchor + 1) % len(od.anchors)
		defocus(prevAnchor)
		focus(od.anchors[od.focusAnchor])
		return od
	case "k", "<Up>":
		prevAnchor := od.anchors[od.focusAnchor]
		anchors := len(od.anchors)
		od.focusAnchor = (od.focusAnchor + anchors - 1) % anchors
		defocus(prevAnchor)
		focus(od.anchors[od.focusAnchor])
		return od
	case "<Enter>":
		anchor := od.anchors[od.focusAnchor]
		href, err := getAttr(anchor, "href")
		if err == nil {
			name, _ := getAttr(anchor, "name")
			return link(od.c, href, name, od)
		}
		return od
	case "u":
		od.source = !od.source
		return od
	case "U":
		od.p.Raw = !od.p.Raw
		return od
	default:
		panic(e.ID)
	}
	return od
}
