package view

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/werkt/bf-client/client"
	"golang.org/x/net/html"

	//"google.golang.org/genproto/googleapis/rpc/status";
	//"google.golang.org/genproto/googleapis/rpc/code";
	"google.golang.org/genproto/googleapis/longrunning"
)

type document struct {
	a           *client.App
	v           View
	d           *client.Document
	p           *client.Paragraph
	anchors     []*html.Node
	focusAnchor int
	source      bool
	paused      bool
	name        string
	op          *longrunning.Operation
	err         error
	rm          *html.Node
	em          *html.Node
	qo          *html.Node
	r           *html.Node
}

func NewDocument(a *client.App, name string, v View) *document {
	d := client.NewDocument()
	content := `
  <html>
    <head>
      <title></title>
    </head>
    <body>
      <h2>Request Metadata:</h2>
      <ul id="request-metadata"></ul>
      <div id="execute-operation-metadata"></div>
      <div id="queued"></div>
      <ul id="response"></ul>
    </body>
  </html>`
	root, err := html.Parse(strings.NewReader(content))
	if err != nil {
		panic(err)
	}
	d.SetRoot(root)
	client.DocumentSetText(d.Find("title"), name)
	rm := d.Find("#request-metadata")
	em := d.Find("#execute-operation-metadata")
	qo := d.Find("#queued")
	r := d.Find("#response")
	d.Update()

	focusAnchor := 0
	anchors := d.FindAll("a")
	if len(anchors) > 0 {
		focus(anchors[focusAnchor])
	}

	return &document{
		a:           a,
		v:           v,
		d:           d,
		name:        name,
		op:          &longrunning.Operation{},
		p:           client.NewParagraph(),
		anchors:     anchors,
		focusAnchor: focusAnchor,
		rm:          rm,
		em:          em,
		qo:          qo,
		r:           r,
	}
}

func (d document) Render() []ui.Drawable {
	ui.Clear()
	d.p.Title = d.d.Title()
	if d.source {
		d.p.Text = d.d.RenderSource()
	} else {
		d.p.Text = d.d.Render()
	}
	d.p.SetRect(0, 0, 120, 60)
	return []ui.Drawable{d.p}
}

func (d *document) fetch() (*longrunning.Operation, error) {
	ops := longrunning.NewOperationsClient(d.a.Conn)

	return ops.GetOperation(context.Background(), &longrunning.GetOperationRequest{
		Name: d.name,
	})
}

func replaceNodeContent(s string, n *html.Node) {
	client.DocumentSetContent(n, frag(s).node)
}

func correlatedInvocationsId(s string) string {
	anchor := strings.Split(s, "#")
	if len(anchor) == 2 {
		return anchor[1]
	}
	return s
}

func updateRequestMetadata(n *html.Node, rm *reapi.RequestMetadata) {
	// might need some mini spans for items
	// might want to format invocation ids
	content := `
  <li>Tool: %s %s</li>
  <li>Action Id: %s</li>
  <li>Tool Invocation Id: <a href="toolInvocation:%[4]s">%[4]s</a></li>
  <li>Correlated Invocations Id: <a href="correlatedInvocations:%s">%s</a></li>
  <li>Action Mnemonic: %s</li>
  <li>Target Id: %s</li>
  <li>Configuration Id: %s</li>
  `
	content = fmt.Sprintf(content, rm.ToolDetails.ToolName, rm.ToolDetails.ToolVersion, rm.ActionId, rm.ToolInvocationId, correlatedInvocationsId(rm.CorrelatedInvocationsId), rm.CorrelatedInvocationsId, rm.ActionMnemonic, rm.TargetId, rm.ConfigurationId)
	replaceNodeContent(content, n)
}

type node struct {
	node *html.Node
}

func el(tag string) node {
	return node{
		node: &html.Node{
			Type: html.ElementNode,
			Data: tag,
		},
	}
}

func text(s string) node {
	return node{
		node: &html.Node{
			Type: html.TextNode,
			Data: s,
		},
	}
}

func frag(f string) node {
	cs, err := html.ParseFragment(strings.NewReader(f), nil)
	if err != nil {
		panic(err)
	}
	// ParseFragment pumps out <html><head/><body>... wrappers for this content
	// Seems dumb
	el := cs[0].LastChild.FirstChild
	return node{
		node: el,
	}
}

func div() node {
	return el("div")
}

func h2() node {
	return el("h2")
}

func ul() node {
	return el("ul")
}

func li() node {
	return el("li")
}

func (n node) id(id string) node {
	n.node.Attr = append(n.node.Attr, html.Attribute{Key: "id", Val: id})
	return n
}

func (n node) text(s string) node {
	n.appendNode(text(s))
	return n
}

func (n node) appendNode(c node) node {
	if n.node.FirstChild == nil {
		n.node.FirstChild = c.node
	}
	if n.node.LastChild != nil {
		n.node.LastChild.NextSibling = c.node
	}
	var last *html.Node
	for last = c.node; last.NextSibling != nil; last = last.NextSibling {
	}
	n.node.LastChild = last
	return n
}

func (n node) append(s string) {
	n.appendNode(text(s))
}

func (n node) li() node {
	if n.node.Type != html.ElementNode || n.node.Data != "ul" {
		panic("unrecognized li container")
	}
	li := li()
	n.appendNode(li)
	return li
}

func (n node) frag(f string) node {
	c := frag(f)
	n.appendNode(c)
	return n
}

func updateExecutedActionMetadata(em *reapi.ExecutedActionMetadata) node {
	root := div().id("executed-metadata")
	root.appendNode(h2().text("Execution Details"))
	ul := ul().id("execution-details")
	root.appendNode(ul)
	if len(em.Worker) != 0 {
		ul.frag(fmt.Sprintf(`Worker: <a href="worker:%[1]s">%[1]s</a></li>`, em.Worker))
	}
	var qt, wst, wct, ifst, ifct, est, ect, oust, ouct time.Time
	var err error
	if qt, err = ptypes.Timestamp(em.QueuedTimestamp); err != nil {
		ul.append(fmt.Sprintf(err.Error()))
		return root
	}
	if em.WorkerStartTimestamp != nil {
		if wst, err = ptypes.Timestamp(em.WorkerStartTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.WorkerCompletedTimestamp != nil {
		if wct, err = ptypes.Timestamp(em.WorkerCompletedTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.InputFetchStartTimestamp != nil {
		if ifst, err = ptypes.Timestamp(em.InputFetchStartTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.InputFetchCompletedTimestamp != nil {
		if ifct, err = ptypes.Timestamp(em.InputFetchCompletedTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.ExecutionStartTimestamp != nil {
		if est, err = ptypes.Timestamp(em.ExecutionStartTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.ExecutionCompletedTimestamp != nil {
		if ect, err = ptypes.Timestamp(em.ExecutionCompletedTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.OutputUploadStartTimestamp != nil {
		if oust, err = ptypes.Timestamp(em.OutputUploadStartTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	if em.OutputUploadCompletedTimestamp != nil {
		if ouct, err = ptypes.Timestamp(em.OutputUploadCompletedTimestamp); err != nil {
			ul.append(fmt.Sprintf(err.Error()))
			return root
		}
	}
	ul.append(fmt.Sprintf("Queued At: %s", formatTime(qt)))
	var qstall time.Duration
	if wst.Compare(qt) > 0 {
		qstall := wst.Sub(qt)
		ul.append(fmt.Sprintf("Worker Start: %s stalled", qstall))
	}
	var ifstall time.Duration
	if ifst.Compare(wst) > 0 {
		ifstall := ifst.Sub(wst)
		ul.append(fmt.Sprintf("Input Fetch start: %s elapsed, %s stalled", ifst.Sub(qt).String(), ifstall.String()))
		if ifct.Compare(qt) > 0 {
			ul.append(fmt.Sprintf("Input Fetch complete: %s elapsed, took %s", ifct.Sub(qt).String(), ifct.Sub(ifst).String()))
		} else {
			ul.append(fmt.Sprintf("Input Fetch running for %s", time.Now().Sub(wst).String()))
		}
	}
	var estall time.Duration
	if est.Compare(ifct) > 0 {
		estall := est.Sub(ifct)
		ul.append(fmt.Sprintf("Execute Start: %s elapsed, %s stalled", est.Sub(qt).String(), estall.String()))
		if ect.Compare(qt) > 0 {
			ul.append(fmt.Sprintf("Execute Complete: %s elapsed, took %s", ect.Sub(qt).String(), ect.Sub(est).String()))
		} else {
			ul.append(fmt.Sprintf("Execute Running for %s", time.Now().Sub(est).String()))
		}
	} else if ifct.Compare(qt) > 0 {
		ul.append(fmt.Sprintf("Execute Stalled for %s", time.Now().Sub(ifct).String()))
	}
	var oustall time.Duration
	if oust.Compare(ect) > 0 {
		oustall := oust.Sub(ect)
		ul.append(fmt.Sprintf("Output Upload Start: %s elapsed, %s stalled", oust.Sub(qt).String(), oustall.String()))
		if ouct.Compare(qt) > 0 {
			ul.append(fmt.Sprintf("Output Upload Complete: %s elapsed, took %s", ouct.Sub(qt).String(), ouct.Sub(oust).String()))
		} else {
			ul.append(fmt.Sprintf("Output Upload Running for %s", time.Now().Sub(oust).String()))
		}
	} else if oust.Compare(qt) > 0 {
		ul.append(fmt.Sprintf("Output Upload Stalled for %s", time.Now().Sub(ect).String()))
	}
	if wct.Compare(qt) > 0 {
		ul.append(fmt.Sprintf(
			"Worker Complete: %s elapsed, %s total stalled",
			wct.Sub(qt).String(),
			(wct.Sub(ouct) + oustall + estall + ifstall + qstall).String()))
	}
	return root
}

func renderDigest(d reapi.Digest, df reapi.DigestFunction_Value) string {
	return client.DigestString(bfpb.Digest{
		Hash:           d.Hash,
		Size:           d.SizeBytes,
		DigestFunction: df,
	})
}

func updateExecuteOperationMetadata(n *html.Node, em *reapi.ExecuteOperationMetadata) {
	content := `
  <div id="stage">Stage: <span id="stage">%s</span></div>
  <div>Action: <a id="action" href="action:%[2]s">%[2]s</a></div>
  `
	actionDigest := renderDigest(*em.ActionDigest, em.DigestFunction)
	content = fmt.Sprintf(content, em.Stage, actionDigest)
	replaceNodeContent(content, n)
	if em.PartialExecutionMetadata != nil {
		// single element return
		n.LastChild.NextSibling = updateExecutedActionMetadata(em.PartialExecutionMetadata).node
		n.LastChild = n.LastChild.NextSibling
	}
}

func updateActionResult(n *html.Node, ar *reapi.ActionResult, df reapi.DigestFunction_Value) {
	el := node{node: n}
	e := "success"
	if ar.ExitCode != 0 {
		e = "failure"
	}
	el.li().frag(fmt.Sprintf(`Exit Code: <span class="exit-%s">%d</span>`, e, ar.ExitCode))
	if len(ar.StdoutRaw) > 0 || (ar.StdoutDigest != nil && ar.StdoutDigest.SizeBytes > 0) {
		digest := renderDigest(*ar.StdoutDigest, df)
		el.li().frag(fmt.Sprintf(`stdout: <a href="file:%[1]s">%[1]s</a>`, digest))
	}
	if len(ar.StderrRaw) > 0 || (ar.StderrDigest != nil && ar.StderrDigest.SizeBytes > 0) {
		digest := renderDigest(*ar.StderrDigest, df)
		el.li().frag(fmt.Sprintf(`stderr: <a href="file:%[1]s">%[1]s</a>`, digest))
	}
	for _, of := range ar.OutputFiles {
		path := renderPath(of.Path, of.IsExecutable)
		digest := renderDigest(*of.Digest, df)
		el.li().frag(fmt.Sprintf(`file: %s (<a href="file:%[2]s">%[2]s</a>)`, path, digest))
	}
	for _, ofs := range ar.OutputFileSymlinks {
		el.li().text(fmt.Sprintf(`symlink: %s -> %s`, ofs.Path, ofs.Target))
	}
	for _, od := range ar.OutputDirectories {
		digest := renderDigest(*od.TreeDigest, df)
		el.li().frag(fmt.Sprintf(`directory: <a href="directory:%[2]s">%[1]s (%[2]s)</a>`, od.Path, digest))
	}
	el.li().appendNode(updateExecutedActionMetadata(ar.ExecutionMetadata))
}

/*
func statusClass(s *status.Status) string {
  if s.Code == code.OK {
    return "ok"
  }
  return "failure"
}
*/

func updateExecuteResponse(n *html.Node, er *reapi.ExecuteResponse, df reapi.DigestFunction_Value) {
	el := node{node: n}
	if er.Result != nil {
		ar := ul()
		el.appendNode(ar)
		updateActionResult(ar.node, er.Result, df)
	}
	s := proto.MarshalTextString(er.Status)
	if len(s) > 0 {
		el.li().text(s)
	}
	el.li().text(fmt.Sprintf(`Served from Cache: %v`, er.CachedResult))
	if len(er.Message) > 0 {
		el.li().text(fmt.Sprintf(`Message: %v`, er.Message))
	}
}

func href(n *html.Node) string {
	for _, a := range n.Attr {
		if a.Key == "href" {
			return a.Val
		}
	}
	return ""
}

func (d *document) Update() {
	if !d.paused && (d.err != nil || !d.op.Done) {
		d.a.Fetches++
		d.op, d.err = d.fetch()

		// need to create fewer new nodes per iteration

		if d.err != nil {
			panic(d.err)
		}
		rm := client.RequestMetadata(d.op)
		if rm != nil {
			updateRequestMetadata(d.rm, rm)
		}
		em, err := client.ExecuteOperationMetadata(d.op)
		if err != nil {
			panic(err)
		}
		updateExecuteOperationMetadata(d.em, em)
		qm := &bfpb.QueuedOperationMetadata{}
		m := d.op.Metadata
		if ptypes.Is(d.op.Metadata, qm) {
			if err := ptypes.UnmarshalAny(m, qm); err != nil {
				panic(err)
			}
			if qm.QueuedOperationDigest != nil {
				replaceNodeContent(fmt.Sprintf(`Queued Operation: <a href="queuedOperation:%[1]s">%[1]s</a>`, client.DigestString(*qm.QueuedOperationDigest)), d.qo)
			}
		}

		df := em.DigestFunction
		switch r := d.op.Result.(type) {
		case *longrunning.Operation_Error:
			replaceNodeContent("error: "+proto.MarshalTextString(r.Error), d.r)
		case *longrunning.Operation_Response:
			er := &reapi.ExecuteResponse{}
			if ptypes.Is(r.Response, er) {
				if err := ptypes.UnmarshalAny(r.Response, er); err != nil {
					panic(err)
				}
				updateExecuteResponse(d.r, er, df)
			}
		}

		anchors := d.d.FindAll("a")
		if len(d.anchors) > 0 {
			a := d.anchors[d.focusAnchor]
			d.focusAnchor = -1
			for i, da := range anchors {
				// crude
				if href(da) == href(a) {
					d.focusAnchor = i
				}
			}
			if d.focusAnchor == -1 {
				// maybe figure out how to jump back to our link...
				d.focusAnchor = 0
			}
		}
		d.anchors = anchors
		focus(d.anchors[d.focusAnchor])
		d.d.Update()
	}
}

func assign(n *html.Node, ns string, key string, val string) {
	for i, attr := range n.Attr {
		if attr.Namespace == ns && attr.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Namespace: ns, Key: key, Val: val})
}

func defocus(n *html.Node) {
	assign(n, "pseudo-class", "focus-visible", "false")
}

func focus(n *html.Node) {
	assign(n, "pseudo-class", "focus-visible", "true")
}

func (d *document) link(target string) View {
	c := strings.SplitN(target, ":", 2)
	view, id := c[0], c[1]
	switch view {
	case "action":
		return NewAction(d.a, client.ParseDigest(id), d)
	case "queuedOperation":
		return d
	case "toolInvocation":
		olv := NewOperationList(d.a, 4, d)
		olv.Filter = "toolInvocationId=" + id
		return olv
	case "correlatedInvocations":
		olv := NewOperationList(d.a, 4, d)
		olv.Filter = "correlatedInvocationsId=" + id
		olv.Name = "toolInvocations"
		return olv
	case "worker":
		return NewWorker(d.a, id, d)
	case "file":
		return d
	}
	return d
}

func getAttr(n *html.Node, k string) (string, error) {
	for _, attr := range n.Attr {
		if attr.Namespace == "" && attr.Key == k {
			return attr.Val, nil
		}
	}
	return "", errors.New("Missing attribute: " + k)
}

func (d *document) Handle(e ui.Event) View {
	switch e.ID {
	case "<Tab>", "j", "<Down>":
		prevAnchor := d.anchors[d.focusAnchor]
		d.focusAnchor = (d.focusAnchor + 1) % len(d.anchors)
		defocus(prevAnchor)
		focus(d.anchors[d.focusAnchor])
		return d
	case "k", "<Up>":
		prevAnchor := d.anchors[d.focusAnchor]
		anchors := len(d.anchors)
		d.focusAnchor = (d.focusAnchor + anchors - 1) % anchors
		defocus(prevAnchor)
		focus(d.anchors[d.focusAnchor])
		return d
	case "<Enter>":
		anchor := d.anchors[d.focusAnchor]
		href, err := getAttr(anchor, "href")
		if err == nil {
			return d.link(href)
		}
		return d
	case "u":
		d.source = !d.source
		return d
	case "U":
		d.p.Raw = !d.p.Raw
		return d
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return d.v
	}
	return d
}
