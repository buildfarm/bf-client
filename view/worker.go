package view

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"github.com/dustin/go-humanize"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/golang/protobuf/ptypes"
	"github.com/werkt/bf-client/client"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type worker struct {
	a       *client.App
	v       View
	w       string
	profile *bfpb.WorkerProfileMessage
	title   *widgets.Paragraph
	// TODO make into array with better types
	match              *client.List
	inputFetch         *client.List
	execute            *client.List
	reportResult       *client.List
	matchPaused        bool
	inputFetchPaused   bool
	executePaused      bool
	reportResultPaused bool
	field              int
	reversed           bool
	fetches            map[string]time.Time
}

func NewStageList() *client.List {
	list := client.NewList()
	list.SelectedRow = -1
	list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	return list
}

func NewWorker(a *client.App, w string, v View) *worker {
	title := widgets.NewParagraph()
	match := NewStageList()
	inputFetch := NewStageList()
	execute := NewStageList()
	reportResult := NewStageList()
	execute.SelectedRow = 0
	return &worker{
		a:            a,
		v:            v,
		w:            w,
		title:        title,
		match:        match,
		inputFetch:   inputFetch,
		execute:      execute,
		reportResult: reportResult,
		fetches:      make(map[string]time.Time),
		profile:      &bfpb.WorkerProfileMessage{},
	}
}

func (v *worker) currentOperationName() string {
	lists := []*client.List{v.match, v.inputFetch, v.execute, v.reportResult}
	for _, list := range lists {
		if list.SelectedRow != -1 && list.SelectedRow < len(list.Rows) {
			return list.Rows[list.SelectedRow].(*stageEx).name
		}
	}
	return ""
}

func (v *worker) cancelOperation() {
	name := v.currentOperationName()
	if len(name) != 0 {
		ops := longrunning.NewOperationsClient(v.a.Conn)
		_, err := ops.CancelOperation(context.Background(), &longrunning.CancelOperationRequest{
			Name: name,
		})
		if err != nil {
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.Unknown {
				panic(err)
			}
			// buildfarm spits out an unknown for already-done
		}
	}
}

func (v *worker) selectedList() *client.List {
	lists := []*client.List{v.match, v.inputFetch, v.execute, v.reportResult}
	for _, list := range lists {
		if list.SelectedRow != -1 {
			return list
		}
	}
	return nil
}

func (v *worker) stageProfile(name string) *bfpb.StageInformation {
	for _, stage := range v.profile.Stages {
		if stage.Name == name {
			return stage
		}
	}
	return nil
}

func (v *worker) currentWidth() int32 {
	if v.inputFetch.SelectedRow != -1 {
		return v.stageProfile("InputFetchStage").SlotsConfigured
	}
	if v.execute.SelectedRow != -1 {
		return v.stageProfile("ExecuteActionStage").SlotsConfigured
	}
	if v.reportResult.SelectedRow != -1 {
		return v.stageProfile("ReportResultStage").SlotsConfigured
	}
	return 1
}

func (v *worker) selectedStage() (string, bool) {
	if v.reportResult.SelectedRow != -1 {
		return "ReportResultStage", v.reportResultPaused
	}
	if v.execute.SelectedRow != -1 {
		return "ExecuteActionStage", v.executePaused
	}
	if v.inputFetch.SelectedRow != -1 {
		return "InputFetchStage", v.inputFetchPaused
	}
	// default here because it's more sane than the rest
	return "MatchStage", v.matchPaused
}

func (v *worker) togglePause() {
	conn := v.a.GetWorkerConn(v.w, v.a.CA)
	c := bfpb.NewWorkerControlClient(conn)
	stage, paused := v.selectedStage()
	paused = !paused
	r, err := c.PipelineChange(context.Background(), &bfpb.WorkerPipelineChangeRequest{
		Changes: []*bfpb.PipelineChange{
			&bfpb.PipelineChange{
				Stage:  stage,
				Paused: paused,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	for _, change := range r.Changes {
		if change.Stage == stage && change.Paused != paused {
			panic("pipeline close not effective")
		}
	}
}

func (v *worker) increaseWidth() {
	v.changeWidth(v.currentWidth() + 1)
}

func (v *worker) decreaseWidth() {
	width := v.currentWidth()
	if width > 1 {
		v.changeWidth(v.currentWidth() - 1)
	}
}

func (v *worker) changeWidth(width int32) {
	conn := v.a.GetWorkerConn(v.w, v.a.CA)
	c := bfpb.NewWorkerControlClient(conn)
	stage, paused := v.selectedStage()
	r, err := c.PipelineChange(context.Background(), &bfpb.WorkerPipelineChangeRequest{
		Changes: []*bfpb.PipelineChange{
			&bfpb.PipelineChange{
				Stage:  stage,
				Paused: paused,
				Width:  width,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	for _, change := range r.Changes {
		if change.Stage == stage && change.Paused != paused {
			panic("pipeline width change not effective")
		}
	}
}

func (v *worker) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return v.v
	case "X":
		v.cancelOperation()
	case "<Enter>":
		return NewDocument(v.a, v.currentOperationName(), v)
	case "j", "<Down>":
		v.selectedList().ScrollDown()
	case "k", "<Up>":
		v.selectedList().ScrollUp()
	case "l", "<Right>":
		v.field++
		v.field %= 4
	case "h", "<Left>":
		v.field += 3
		v.field %= 4
	case "<Tab>":
		if v.match.SelectedRow != -1 {
			v.match.SelectedRow = -1
			v.inputFetch.SelectedRow = 0
		} else if v.inputFetch.SelectedRow != -1 {
			v.inputFetch.SelectedRow = -1
			v.execute.SelectedRow = 0
		} else if v.execute.SelectedRow != -1 {
			v.execute.SelectedRow = -1
			v.reportResult.SelectedRow = 0
		} else {
			v.reportResult.SelectedRow = -1
			v.match.SelectedRow = 0
		}
	case ">", "<":
		v.reversed = !v.reversed
	case "P":
		v.togglePause()
	case "+":
		v.increaseWidth()
	case "-":
		v.decreaseWidth()
	}
	return v
}

type stageEx struct {
	field    func() int
	name     string
	stalled  bool
	errored  bool
	fence    time.Time
	final    time.Time
	target   string
	mnemonic string
	build    string
	done     bool
}

func (e stageEx) label() string {
	switch e.field() {
	case 1:
		return e.target
	case 2:
		return e.mnemonic
	case 3:
		return e.build
	}
	return e.name
}

func reasonableDuration(d time.Duration) time.Duration {
	if d.Minutes() >= 1 {
		return d.Truncate(1 * time.Second)
	}
	return d.Truncate(1 * time.Millisecond)
}

func (e stageEx) String() string {
	if e.errored {
		return fmt.Sprintf("[%s](fg:black,bg:red)", e.label())
	}
	// needs a clock inject
	label := e.label()
	if !e.fence.IsZero() {
		end := time.Now()
		if e.done {
			end = e.final
		}
		label += " " + reasonableDuration(end.Sub(e.fence)).String()
	}
	if e.stalled {
		label = fmt.Sprintf("[%s](fg:black,bg:blue)", label)
	}
	return label
}

func selectedTitle(s bool, t string) string {
	if s {
		return ">" + t + "<"
	}
	return t
}

func filterEmpty(r []string) []string {
	// have to filter for empty string, which match can be
	names := r[:0]
	for _, name := range r {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

type byExec func(e1, e2 fmt.Stringer) bool

func (by byExec) Sort(executions []fmt.Stringer) {
	es := &execSorter{
		executions: executions,
		by:         by,
	}
	sort.Sort(es)
}

type execSorter struct {
	executions []fmt.Stringer
	by         byExec // func(e1, e2 *string) bool // Closure used in the Less method.
}

func (s *execSorter) Len() int {
	return len(s.executions)
}

func (s *execSorter) Swap(i, j int) {
	s.executions[i], s.executions[j] = s.executions[j], s.executions[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *execSorter) Less(i, j int) bool {
	return !s.by(s.executions[i], s.executions[j])
}

func (v *worker) populateExecutions(l *client.List, stage string, r []string) {
	r = filterEmpty(r)
	rows := make([]fmt.Stringer, len(r))
	for i, name := range r {
		ex := &stageEx{field: func() int { return v.field }, name: name}
		op, ok := v.a.Ops[name]
		if ok {
			stalled, fence, err := stageFenced(op, stage)
			if err != nil {
				// non-result complete operation
				ex.errored = true
			} else {
				ex.stalled, ex.fence = stalled, fence
			}
			m := client.RequestMetadata(op)
			if m == nil {
				panic(op)
			}
			ex.target = m.TargetId
			ex.mnemonic = m.ActionMnemonic
			ex.build = m.CorrelatedInvocationsId
		}
		rows[i] = ex
	}
	fence := func(e1, e2 fmt.Stringer) bool {
		stageEx1, stageEx2 := e1.(*stageEx), e2.(*stageEx)
		if stageEx1.errored != stageEx2.errored {
			return stageEx2.errored != v.reversed
		}
		if stageEx1.stalled != stageEx2.stalled {
			return stageEx2.stalled != v.reversed
		}
		return (stageEx1.fence.Compare(stageEx2.fence) < 0) != v.reversed
	}
	byExec(fence).Sort(rows)
	l.Rows = rows
}

func opMatchesStage(op *longrunning.Operation, stage *bfpb.StageInformation) (bool, error) {
	em, err := client.ExecuteOperationMetadata(op)
	if err != nil {
		return false, err
	}
	switch stage.Name {
	case "MatchStage", "InputFetchStage":
		return em.Stage == reapi.ExecutionStage_QUEUED, nil
	case "ExecuteActionStage":
		return em.Stage == reapi.ExecutionStage_EXECUTING, nil
	case "ReportResultStage":
		return em.Stage == reapi.ExecutionStage_COMPLETED, nil
	}
	return false, errors.New("Unknown stage: " + stage.Name)
}

func stageFenced(op *longrunning.Operation, stage string) (bool, time.Time, error) {
	eam, err := client.ExecutedActionMetadata(op)
	if err == nil && eam == nil {
		err = errors.New(fmt.Sprintf("%s: %s, ExecutedActionMetadata is nil", stage, op.Name))
	}
	if err != nil {
		return false, time.Time{}, err
	}
	var st, ct time.Time
	stalled := false
	switch stage {
	case "MatchStage":
		st, _ = ptypes.Timestamp(eam.WorkerStartTimestamp)
		ct, _ = ptypes.Timestamp(eam.InputFetchStartTimestamp)
	case "InputFetchStage":
		st, _ = ptypes.Timestamp(eam.InputFetchStartTimestamp)
		ct, _ = ptypes.Timestamp(eam.InputFetchCompletedTimestamp)
	case "ExecuteActionStage":
		st, _ = ptypes.Timestamp(eam.ExecutionStartTimestamp)
		ct, _ = ptypes.Timestamp(eam.ExecutionCompletedTimestamp)
	case "ReportResultStage":
		st, _ = ptypes.Timestamp(eam.OutputUploadStartTimestamp)
		ct, _ = ptypes.Timestamp(eam.OutputUploadCompletedTimestamp)
	}
	fence := st
	if ct.Compare(st) > 0 {
		stalled = true
		fence = ct
	}
	return stalled, fence, nil
}

func getExecution(a *client.App, name string, conn *grpc.ClientConn, wg *sync.WaitGroup) {
	defer wg.Done()

	c := longrunning.NewOperationsClient(conn)
	o, err := c.GetOperation(context.Background(), &longrunning.GetOperationRequest{
		Name: name,
	})
	if err == nil {
		a.Mutex.Lock()
		a.Ops[name] = o
		a.Mutex.Unlock()
	}
}

func (v worker) Render() []ui.Drawable {
	v.title.Text = fmt.Sprintf(
		"%s CAS Count: %d Size: %s (%d%%) Unref: %d%%",
		v.w, v.profile.CasEntryCount, humanize.Bytes(uint64(v.profile.CasSize)),
		int((float64(v.profile.CasSize)/float64(v.profile.CasMaxSize))*100),
		int((float64(v.profile.CasUnreferencedEntryCount)/float64(v.profile.CasEntryCount))*100))
	v.title.Border = false
	v.title.SetRect(0, -1, 80, 2)
	v.match.Title = selectedTitle(v.match.SelectedRow != -1, "Match")

	v.reportResult.Title = selectedTitle(v.reportResult.SelectedRow != -1, "ReportResult")

	// need some expander logic

	now := time.Now()

	v.a.Fetches = 0

	fetches := make([]string, 0)
	for _, stage := range v.profile.Stages {
		// for all operations in stages
		for _, name := range stage.OperationNames {
			// if operation not in cache, fetch it
			fetch := false
			if name == "" {
				continue
			}
			if op, ok := v.a.Ops[name]; ok && op != nil {
				match, err := opMatchesStage(op, stage)
				if err != nil {
					panic(err)
				}
				if match {
					stalled, fence, _ := stageFenced(op, stage.Name)
					if !stalled {
						fenced := now.Sub(fence)
						if last, ok := v.fetches[name]; ok {
							deadline := 1.0
							if fenced.Minutes() >= 1 {
								deadline = 10.0
							}
							match = now.Sub(last).Seconds() < deadline
						}
					}
				}
				fetch = !match
			} else {
				fetch = true
				// if stage is not the operation current stage, fetch it
			}
			if fetch {
				v.a.Fetches++
				v.fetches[name] = now
				fetches = append(fetches, name)
			}
		}
		wg := sync.WaitGroup{}
		wg.Add(len(fetches))
		for _, name := range fetches {
			go getExecution(v.a, name, v.a.Conn, &wg)
		}
		wg.Wait() // really needs to move to Update

		switch stage.Name {
		case "MatchStage":
			v.populateExecutions(v.match, stage.Name, stage.OperationNames)
		case "InputFetchStage":
			v.inputFetch.Title = fmt.Sprintf(selectedTitle(v.inputFetch.SelectedRow != -1, "InputFetch")+" %d/%d", stage.SlotsUsed, stage.SlotsConfigured)
			v.populateExecutions(v.inputFetch, stage.Name, stage.OperationNames)
		case "ExecuteActionStage":
			executions := len(stage.OperationNames)
			avg_slots_per_execution := float32(0)
			if executions > 0 {
				avg_slots_per_execution = float32(stage.SlotsUsed) / float32(executions)
			}
			v.execute.Title = fmt.Sprintf(selectedTitle(v.execute.SelectedRow != -1, "Execute")+" %d/%d (%d) Avg %g", stage.SlotsUsed, stage.SlotsConfigured, executions, avg_slots_per_execution)
			// TODO get some time spent doing this
			v.populateExecutions(v.execute, stage.Name, stage.OperationNames)
		case "ReportResultStage":
			v.reportResult.Title = fmt.Sprintf(selectedTitle(v.reportResult.SelectedRow != -1, "Report Result")+" %d/%d", stage.SlotsUsed, stage.SlotsConfigured)
			v.populateExecutions(v.reportResult, stage.Name, stage.OperationNames)
		}
	}

	row := 1
	v.match.SetRect(0, row, 80, row+3)
	row += 3
	// inputFetchRows := len(v.inputFetch.Rows)
	inputFetchHeight := 5
	v.inputFetch.SetRect(0, row, 80, row+2+inputFetchHeight)
	row += 2 + inputFetchHeight
	// executeRows := len(v.execute.Rows)
	executeHeight := 5
	v.execute.SetRect(0, row, 80, row+2+executeHeight)
	row += 2 + executeHeight
	// reportResultRows := len(v.reportResult.Rows)
	reportResultHeight := 5
	v.reportResult.SetRect(0, row, 80, row+2+reportResultHeight)

	return []ui.Drawable{v.title, v.match, v.inputFetch, v.execute, v.reportResult}
}

func pausedStyle(p bool) ui.Style {
	if p {
		return ui.NewStyle(ui.ColorRed)
	} else {
		return ui.Theme.Block.Border
	}
}

func (v *worker) Update() {
	conn := v.a.GetWorkerConn(v.w, v.a.CA)
	workerProfile := bfpb.NewWorkerProfileClient(conn)
	profile, err := workerProfile.GetWorkerProfile(context.Background(), &bfpb.WorkerProfileRequest{})
	if err == nil {
		v.profile = profile
	}
	c := bfpb.NewWorkerControlClient(conn)
	r, err := c.PipelineChange(context.Background(), &bfpb.WorkerPipelineChangeRequest{})
	if err != nil {
		panic(err)
	}
	for _, change := range r.Changes {
		switch change.Stage {
		case "MatchStage":
			v.matchPaused = change.Paused
			v.match.BorderStyle = pausedStyle(change.Paused)
		case "InputFetchStage":
			v.inputFetchPaused = change.Paused
			v.inputFetch.BorderStyle = pausedStyle(change.Paused)
		case "ExecuteActionStage":
			v.executePaused = change.Paused
			v.execute.BorderStyle = pausedStyle(change.Paused)
		case "ReportResultStage":
			v.reportResultPaused = change.Paused
			v.reportResult.BorderStyle = pausedStyle(change.Paused)
		}
	}
}
