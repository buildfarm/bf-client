package view

import (
	"container/list"
	"context"
	"fmt"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/werkt/bf-client/client"
	"google.golang.org/genproto/googleapis/longrunning"
	"regexp"
	"time"
)

type operationView struct {
	a                *client.App
	name             string
	op               *longrunning.Operation
	err              error
	v                View
	selection        int // enum of correlated, invocation, action
	selectableFields int
	selectionActions []func(*operationView) View
	paused           bool
	p                *widgets.Paragraph
}

func NewOperation(a *client.App, name string, v View) *operationView {
	return &operationView{
		a:                a,
		name:             name,
		op:               &longrunning.Operation{},
		v:                v,
		selection:        0,
		selectableFields: 0,
		p:                widgets.NewParagraph(),
	}
}

func (v *operationView) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return v.v
	case "j", "<Down>":
		v.selection++
		if v.selectableFields > 0 {
			v.selection %= v.selectableFields
		} else {
			v.selection = 0
		}
	case "k", "<Up>":
		v.selection += v.selectableFields - 1
		if v.selectableFields > 0 {
			v.selection %= v.selectableFields
		} else {
			v.selection = 0
		}
	case "p", "<Space>":
		v.paused = !v.paused
	case "<Enter>":
		if v.selectableFields > 0 {
			return v.selectionActions[v.selection](v)
		}
	}
	return v
}

func (v *operationView) fetch() (*longrunning.Operation, error) {
	ops := longrunning.NewOperationsClient(v.a.Conn)

	return ops.GetOperation(context.Background(), &longrunning.GetOperationRequest{
		Name: v.name,
	})
}

func (v *operationView) Update() {
	if !v.paused && (v.err != nil || !v.op.Done) {
		v.a.Fetches++
		v.op, v.err = v.fetch()
	}
}

func (v *operationView) Render() []ui.Drawable {
	p := v.p
	p.Title = v.name
	if v.paused {
		p.Title += " (Paused)"
	}
	p.WrapText = true
	if v.err != nil {
		p.Text = string(v.err.Error())
		v.selectableFields = 0
	} else if !v.paused {
		p.Text = v.renderOperation()
	}
	p.SetRect(0, 0, 120, 60)

	return []ui.Drawable{p}
}

func (v *operationView) renderOperation() string {
	m := v.op.Metadata
	em := &reapi.ExecuteOperationMetadata{}
	qm := &bfpb.QueuedOperationMetadata{}
	df := reapi.DigestFunction_UNKNOWN
	var text string
	if ptypes.Is(m, em) {
		if err := ptypes.UnmarshalAny(m, em); err != nil {
			text = err.Error()
			v.selectableFields = 0
		} else {
			df = em.DigestFunction
			m, ok := v.a.Metadatas[v.name]
			text = ""
			if ok {
				text += renderRequestMetadata(m, v.selection)
				v.selectableFields = 3
			} else {
				v.selectableFields = 1
			}
			text += renderExecuteOperationMetadata(em, v.selection)
			v.selectionActions = make([]func(*operationView) View, v.selectableFields)
			actionIndex := 0
			if ok {
				v.selectionActions[0] = createInvocationView
				v.selectionActions[1] = createCorrelatedView
				actionIndex = 2
			}
			v.selectionActions[actionIndex] = func(ov *operationView) View { return NewAction(v.a, client.ToDigest(*em.ActionDigest, df), ov) }
		}
	} else if ptypes.Is(m, qm) {
		if err := ptypes.UnmarshalAny(m, qm); err != nil {
			text = err.Error()
			v.selectableFields = 0
		} else {
			df = qm.ExecuteOperationMetadata.DigestFunction
			text = renderRequestMetadata(qm.RequestMetadata, v.selection)
			text += renderExecuteOperationMetadata(qm.ExecuteOperationMetadata, v.selection)
			text += fmt.Sprintf("queued operation: %s\n", xrenderDigest(*qm.QueuedOperationDigest, v.selection == 3))
			v.selectableFields = 4
			v.selectionActions = make([]func(*operationView) View, v.selectableFields)
			v.selectionActions[0] = createInvocationView
			v.selectionActions[1] = createCorrelatedView
			v.selectionActions[2] = func(ov *operationView) View {
				return NewAction(v.a, client.ToDigest(*qm.ExecuteOperationMetadata.ActionDigest, df), ov)
			}
			v.selectionActions[3] = func(ov *operationView) View {
				return createQueuedOperationView(ov, qm.QueuedOperationDigest)
			}
		}
	} else {
		text = proto.MarshalTextString(v.op)
		v.selectableFields = 0
	}

	switch r := v.op.Result.(type) {
	case *longrunning.Operation_Error:
		text += "error: " + proto.MarshalTextString(r.Error)
	case *longrunning.Operation_Response:
		er := &reapi.ExecuteResponse{}
		if ptypes.Is(r.Response, er) {
			if err := ptypes.UnmarshalAny(r.Response, er); err != nil {
				text += err.Error()
				v.selectableFields = 0
			} else {
				ex := renderExecuteResponse(er, df, v.selection)
				text += ex.text
				selectableFields := v.selectableFields
				v.selectableFields += ex.fields
				selectionActions := v.selectionActions
				v.selectionActions = make([]func(*operationView) View, v.selectableFields)
				for i := 0; i < selectableFields; i++ {
					v.selectionActions[i] = selectionActions[i]
				}
				for e, i := ex.actions.Front(), selectableFields; e != nil; e = e.Next() {
					v.selectionActions[i] = e.Value.(func(v *operationView) View)
					i++
				}
			}
		}
	}
	return text
}

type executeText struct {
	text    string
	fields  int
	actions *list.List
}

func renderExecuteResponse(er *reapi.ExecuteResponse, df reapi.DigestFunction_Value, selection int) executeText {
	var ex executeText
	if er.Result != nil {
		ex = renderActionResult(er.Result, df, selection)
	} else {
		ex = executeText{
			text:    "nil action result\n",
			fields:  0,
			actions: list.New(),
		}
	}
	text := ex.text
	text += proto.MarshalTextString(er.Status)
	text += fmt.Sprintf("served from cache: %v\n", er.CachedResult)
	// server logs...
	if len(er.Message) > 0 {
		text += "message: " + er.Message + "\n"
	}
	return executeText{
		text:    text,
		fields:  ex.fields,
		actions: ex.actions,
	}
}

func renderRequestMetadata(rm *reapi.RequestMetadata, selection int) string {
	metadataSelections := [...]string{"tool_invocation_id", "correlated_invocations_id"}
	text := "request metadata: " + proto.MarshalTextString(rm)
	if selection < 2 {
		field := metadataSelections[selection]
		match := field + ": (.*)"
		re := regexp.MustCompile(match)
		text = re.ReplaceAllString(text, field+": [$1](mod:bold)")
	}
	return text
}

func renderActionResult(ar *reapi.ActionResult, df reapi.DigestFunction_Value, selection int) executeText {
	text := fmt.Sprintf("exit code: %d\n", ar.ExitCode)
	base := 0
	actions := list.New()
	if len(ar.StderrRaw) > 0 || (ar.StdoutDigest != nil && ar.StdoutDigest.SizeBytes > 0) {
		text += fmt.Sprintf("stdout: %s%s\n", renderREDigest(*ar.StdoutDigest, df, selection == 3), renderInline(len(ar.StdoutRaw)))
		base++
		actions.PushBack(func(v *operationView) View { return contentView(v, ar.StdoutDigest, ar.StdoutRaw, "stdout") })
	}
	if len(ar.StderrRaw) > 0 || (ar.StderrDigest != nil && ar.StderrDigest.SizeBytes > 0) {
		text += fmt.Sprintf("stderr: %s%s\n", renderREDigest(*ar.StderrDigest, df, selection == 4), renderInline(len(ar.StderrRaw)))
		base++
		actions.PushBack(func(v *operationView) View { return contentView(v, ar.StderrDigest, ar.StderrRaw, "stderr") })
	}
	for i, of := range ar.OutputFiles {
		path := renderPath(of.Path, of.IsExecutable)
		text += fmt.Sprintf("file: %s (%s)%s\n", path, renderREDigest(*of.Digest, df, selection == base+i), renderInline(len(of.Contents)))
		actions.PushBack(func(v *operationView) View { return contentView(v, of.Digest, nil, path) })
	}
	base += len(ar.OutputFiles)
	for _, ofs := range ar.OutputFileSymlinks {
		text += fmt.Sprintf("symlink: %s -> %s\n", ofs.Path, ofs.Target)
	}
	for i, od := range ar.OutputDirectories {
		text += fmt.Sprintf("directory: %s (%s)\n", od.Path, renderREDigest(*od.TreeDigest, df, selection == base+i))
		actions.PushBack(func(v *operationView) View { return outputDirectoryView(v, od.TreeDigest, od.Path) })
	}
	base += len(ar.OutputDirectories)
	text += renderExecutedActionMetadata(ar.ExecutionMetadata, selection == base)
	fields := base + 1
	return executeText{
		text:    text,
		fields:  fields,
		actions: actions,
	}
}

func renderExecutedActionMetadata(em *reapi.ExecutedActionMetadata, workerSelected bool) string {
	text := ""
	if len(em.Worker) != 0 {
		text = fmt.Sprintf("worker: %s\n", boldIfSelected(em.Worker, workerSelected))
	}
	var qt, wst, wct, ifst, ifct, est, ect, oust, ouct time.Time
	var err error
	if err = em.QueuedTimestamp.CheckValid(); err != nil {
		text += err.Error() + "\n"
		return text
	}
	qt = em.QueuedTimestamp.AsTime()
	if em.WorkerStartTimestamp != nil {
		if err = em.WorkerStartTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		wst = em.WorkerStartTimestamp.AsTime()
	}
	if em.WorkerCompletedTimestamp != nil {
		if err = em.WorkerCompletedTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		wct = em.WorkerCompletedTimestamp.AsTime()
	}
	if em.InputFetchStartTimestamp != nil {
		if err = em.InputFetchStartTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		ifst = em.InputFetchStartTimestamp.AsTime()
	}
	if em.InputFetchCompletedTimestamp != nil {
		if err = em.InputFetchCompletedTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		ifct = em.InputFetchCompletedTimestamp.AsTime()
	}
	if em.ExecutionStartTimestamp != nil {
		if err = em.ExecutionStartTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		est = em.ExecutionStartTimestamp.AsTime()
	}
	if em.ExecutionCompletedTimestamp != nil {
		if err = em.ExecutionCompletedTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		ect = em.ExecutionCompletedTimestamp.AsTime()
	}
	if em.OutputUploadStartTimestamp != nil {
		if err = em.OutputUploadStartTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		oust = em.OutputUploadStartTimestamp.AsTime()
	}
	if em.OutputUploadCompletedTimestamp != nil {
		if err = em.OutputUploadCompletedTimestamp.CheckValid(); err != nil {
			text += err.Error() + "\n"
			return text
		}
		ouct = em.OutputUploadCompletedTimestamp.AsTime()
	}
	text += fmt.Sprintf("queued at: %s\n", formatTime(qt))
	var qstall time.Duration
	if wst.Compare(qt) > 0 {
		qstall := wst.Sub(qt)
		text += fmt.Sprintf("worker start: %s stalled\n", qstall)
	}
	var ifstall time.Duration
	if ifst.Compare(wst) > 0 {
		ifstall := ifst.Sub(wst)
		text += fmt.Sprintf("input fetch start: %s elapsed, %s stalled\n", ifst.Sub(qt).String(), ifstall.String())
		if ifct.Compare(qt) > 0 {
			text += fmt.Sprintf("input fetch complete: %s elapsed, took %s\n", ifct.Sub(qt).String(), ifct.Sub(ifst).String())
		} else {
			text += fmt.Sprintf("input fetch running for %s\n", time.Now().Sub(wst).String())
		}
	}
	var estall time.Duration
	if est.Compare(ifct) > 0 {
		estall := est.Sub(ifct)
		text += fmt.Sprintf("execute start: %s elapsed, %s stalled\n", est.Sub(qt).String(), estall.String())
		if ect.Compare(qt) > 0 {
			text += fmt.Sprintf("execute complete: %s elapsed, took %s\n", ect.Sub(qt).String(), ect.Sub(est).String())
		} else {
			text += fmt.Sprintf("execute running for %s\n", time.Now().Sub(est).String())
		}
	} else if ifct.Compare(qt) > 0 {
		text += fmt.Sprintf("execute stalled for %s\n", time.Now().Sub(ifct).String())
	}
	var oustall time.Duration
	if oust.Compare(ect) > 0 {
		oustall := oust.Sub(ect)
		text += fmt.Sprintf("output upload start: %s elapsed, %s stalled\n", oust.Sub(qt).String(), oustall.String())
		if ouct.Compare(qt) > 0 {
			text += fmt.Sprintf("output upload complete: %s elapsed, took %s\n", ouct.Sub(qt).String(), ouct.Sub(oust).String())
		} else {
			text += fmt.Sprintf("output upload running for %s\n", time.Now().Sub(oust).String())
		}
	} else if oust.Compare(qt) > 0 {
		text += fmt.Sprintf("output upload stalled for %s\n", time.Now().Sub(ect).String())
	}
	if wct.Compare(qt) > 0 {
		text += fmt.Sprintf(
			"worker complete: %s elapsed, %s total stalled\n",
			wct.Sub(qt).String(),
			(wct.Sub(ouct) + oustall + estall + ifstall + qstall).String())
	}
	return text
}

func renderExecuteOperationMetadata(em *reapi.ExecuteOperationMetadata, selection int) string {
	stage := &reapi.ExecuteOperationMetadata{
		Stage: em.Stage,
	}
	text := proto.MarshalTextString(stage) + "\n"
	text += fmt.Sprintf("action: %s\n", renderREDigest(*em.ActionDigest, em.DigestFunction, selection == 2))
	if em.PartialExecutionMetadata != nil {
		text += renderExecutedActionMetadata(em.PartialExecutionMetadata, selection == 4)
	}
	return text
}

func renderInline(l int) string {
	if l > 0 {
		return " inline"
	}
	return ""
}

func renderPath(p string, e bool) string {
	if e {
		return "*" + p
	}
	return p
}

func boldIfSelected(s string, selected bool) string {
	if selected {
		return "[" + s + "](mod:bold)"
	}
	return s
}

// deserves its own file probably
func xrenderDigest(d bfpb.Digest, selected bool) string {
	return boldIfSelected(client.DigestString(d), selected)
}

func renderREDigest(d reapi.Digest, df reapi.DigestFunction_Value, selected bool) string {
	return xrenderDigest(bfpb.Digest{
		Hash:           d.Hash,
		Size:           d.SizeBytes,
		DigestFunction: df,
	}, selected)
}

func createCorrelatedView(v *operationView) View {
	return v
}

func createInvocationView(v *operationView) View {
	m, ok := v.a.Metadatas[v.name]

	if !ok {
		panic(v.name)
		return v
	}

	olv := NewOperationList(v.a, 4, v)
	olv.Filter = "invocationId=" + m.ToolInvocationId
	return olv
}

func createQueuedOperationView(v *operationView, d *bfpb.Digest) View {
	return v
}

func outputDirectoryView(v *operationView, d *reapi.Digest, t string) View {
	return v
}

func createWorkerView(v *operationView, w string) View {
	return v
}

func contentView(v *operationView, d *reapi.Digest, c []byte, t string) View {
	return v
}
