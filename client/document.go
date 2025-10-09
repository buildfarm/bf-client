package client

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ericchiang/css"
	"golang.org/x/net/html"
)

type styleNode struct {
	selector   string
	properties map[string]string
}

type Document struct {
	root   *html.Node
	styles []styleNode
}

type render struct {
	d       *Document
	blocked bool
}

var defaultTermuiStyles = []styleNode{
	styleNode{
		selector: "html",
		properties: map[string]string{
			"display": "inline",
		},
	},
	styleNode{
		selector: "head",
		properties: map[string]string{
			"display": "none",
		},
	},
	styleNode{
		selector: "body",
		properties: map[string]string{
			"display": "block",
		},
	},
	styleNode{
		selector: "div",
		properties: map[string]string{
			"display": "block",
		},
	},
	styleNode{
		selector: "ul",
		properties: map[string]string{
			"display": "block",
			"padding": "2",
		},
	},
	styleNode{
		selector: "li",
		properties: map[string]string{
			"display": "list-item",
		},
	},
	styleNode{
		selector: "a",
		properties: map[string]string{
			"text-decoration-line": "underline",
			"color":                "blue",
			"display":              "inline",
		},
	},
	styleNode{
		selector: "a:focus-visible",
		properties: map[string]string{
			"text-decoration-color": "reverse",
		},
	},
	styleNode{
		selector: "h2",
		properties: map[string]string{
			"display":              "block",
			"text-decoration-line": "bold",
		},
	},
}

func defaultStyles() []styleNode {
	return defaultTermuiStyles[:]
}

func NewDocument() *Document {
	return &Document{
		styles: defaultStyles(),
	}
}

func SelectAll(n *html.Node, selector string) []*html.Node {
	sel, err := css.Parse(selector)
	if err != nil {
		panic(err)
	}
	return sel.Select(n)
}

func Select(n *html.Node, selector string) *html.Node {
	nodes := SelectAll(n, selector)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func Show(n *html.Node) {
	// not definitive, could be hidden by style
	n.Attr = slices.DeleteFunc(n.Attr, func(a html.Attribute) bool {
		return a.Key == "style" && a.Val == "display:none"
	})
}

func Hide(n *html.Node) {
	n.Attr = append(n.Attr, html.Attribute{
		Key: "style",
		Val: "display:none",
	})
}

func (d *Document) FindAll(selector string) []*html.Node {
	return SelectAll(d.root, selector)
}

func (d *Document) Find(selector string) *html.Node {
	return Select(d.root, selector)
}

func (d *Document) RenderSource() string {
	var s strings.Builder
	html.Render(&s, d.root)
	return s.String()
}

func (d *Document) Render() string {
	r := render{
		d:       d,
		blocked: false,
	}
	return r.run()
}

func (d *Document) SetRoot(n *html.Node) {
	d.root = n
}

func (d *Document) Update() {
	r := render{
		d: d,
	}
	r.htmlTrim(d.root, false)
}

func (d *Document) Parse(data string) {
	var err error
	d.root, err = html.Parse(strings.NewReader(data))
	if err != nil {
		panic(err)
	}
	r := render{
		d: d,
	}
	r.htmlTrim(d.root, false)
}

func hasPseudoClass(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Namespace == "pseudo-class" && attr.Key == key {
			return attr.Val == "true"
		}
	}
	return false
}

func (r *render) run() string {
	return r.node(r.d.root, make([]styleNode, 0), 0)
}

func parseStyle(s string) styleNode {
	var style styleNode
	// jjjjank
	if s == "display:none" {
		style.properties = map[string]string{
			"display": "none",
		}
	} else {
		style.properties = map[string]string{}
	}
	return style
}

func (r *render) findStyles(n *html.Node) []styleNode {
	styles := make([]styleNode, 0)
	pseudo := ""
	if hasPseudoClass(n, "focus-visible") {
		pseudo = "focus-visible"
	}
	for _, attr := range n.Attr {
		if attr.Key == "style" {
			styles = append(styles, parseStyle(attr.Val))
		}
	}
	for _, style := range r.d.styles {
		// juvenile
		if style.selector == n.Data {
			styles = append(styles, style)
		}
		if pseudo != "" && style.selector == n.Data+":"+pseudo {
			styles = append(styles, style)
		}
	}
	return styles
}

func (r *render) htmlTrim(n *html.Node, deleteLeadingSpace bool) {
	inlineNewline := regexp.MustCompile(`\s*\n[\s\n]*`)
	successiveSpaces := regexp.MustCompile(`[\s\n]+`)
	var last *html.Node = nil
	var first *html.Node = nil
	hasTrailingSpace := false
	isBlock := false
	if n.Type == html.ElementNode {
		if n.Data == "<block>" {
			isBlock = false
		} else {
			for _, style := range r.findStyles(n) {
				display, ok := style.properties["display"]
				isBlock = ok && display == "block"
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		hasTrailingSpace = false
		if c.Type == html.TextNode {
			if isBlock {
				block := html.Node{Type: html.ElementNode, Data: "<block>", FirstChild: c, LastChild: c}
				next := c.NextSibling
				r.htmlTrim(&block, false)
				c.NextSibling = next
			} else {
				if first == nil {
					first = c
				}
				data := string(inlineNewline.ReplaceAll([]byte(c.Data), []byte("\n")))
				data = string(successiveSpaces.ReplaceAll([]byte(data), []byte(" ")))
				if len(data) == 0 {
					continue
				}
				if deleteLeadingSpace && data[0] == ' ' {
					data = data[1:]
					deleteLeadingSpace = false
				}
				c.Data = data
				hasTrailingSpace = data[len(data)-1] == ' '
			}
		} else {
			r.htmlTrim(c, hasTrailingSpace)
		}
		last = c
	}
	if first != nil {
		first.Data = strings.TrimLeft(first.Data, " ")
	}
	if last != nil {
		last.Data = strings.TrimRight(last.Data, " ")
	}
}

func (r *render) styled(n *html.Node, styles []styleNode) string {
	if n.Type == html.TextNode {
		text := n.Data
		if len(text) == 0 {
			return text
		}
		fg, bg, mod, color := "", "", "", ""
		for _, style := range styles {
			var val string
			var ok bool
			val, ok = style.properties["color"]
			if ok {
				fg = val
			}
			val, ok = style.properties["background-color"]
			if ok {
				bg = val
			}
			val, ok = style.properties["text-decoration-line"]
			if ok {
				mod = val
			}
			val, ok = style.properties["text-decoration-color"]
			if ok {
				color = val
			}
		}
		if color == "reverse" {
			if fg == "" {
				fg = "white"
			}
			if bg == "" {
				bg = "black"
			}
			fg, bg = bg, fg
		}
		if fg != "" || bg != "" || mod != "" {
			attr, sep := "", ""
			if fg != "" {
				attr, sep = attr+sep+"fg:"+fg, ","
			}
			if bg != "" {
				attr, sep = attr+sep+"bg:"+bg, ","
			}
			if mod != "" {
				attr, sep = attr+sep+"mod:"+mod, ","
			}
			text = fmt.Sprintf("[%s](%s)", text, attr)
		}
		return text
	}
	return ""
}

func (r *render) node(n *html.Node, styles []styleNode, x int) string {
	block := false
	display := "inline"
	if n.Type != html.TextNode {
		styles = r.findStyles(n)
		for _, style := range styles {
			var ok bool
			display, ok = style.properties["display"]
			if ok {
				block = display == "block" || display == "list-item"
			}
		}
	}
	for _, style := range styles {
		val, ok := style.properties["padding"]
		if ok {
			val, err := strconv.ParseInt(val, 0, 32)
			if err != nil {
				panic(err)
			}
			x += int(val)
		}
	}
	visible := true
	for _, style := range styles {
		visible = style.properties["display"] != "none"
	}
	if !visible {
		return ""
	}
	s := r.styled(n, styles)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if block && (display != "list-item" || c == n.FirstChild) && c.Type == html.TextNode && len(c.Data) != 0 {
			s += "\n" + strings.Repeat(" ", x)
		}
		s += r.node(c, styles, x)
	}
	return s
}

func (d Document) Title() string {
	return d.Find("html > head > title").FirstChild.Data
}

func DocumentSetText(n *html.Node, t string) {
	DocumentSetContent(n, &html.Node{
		Type: html.TextNode,
		Data: t,
	})
}

func DocumentSetContent(n *html.Node, c *html.Node) {
	n.FirstChild = c
	for c = n.FirstChild; c.NextSibling != nil; c = c.NextSibling {
	}
	n.LastChild = c
}
