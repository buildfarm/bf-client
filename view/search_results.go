package view

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	ui "github.com/gizak/termui/v3"
	"github.com/werkt/bf-client/client"
	"google.golang.org/genproto/googleapis/longrunning"
)

type searchResults struct {
	a         *client.App
	v         View
	list      *client.List
	resource  string
	name      string
	filter    string
	fetched   bool
	pageToken string
	opRows    []*opRow

	mode              int
	selectedName      string
	fetchDetailsIndex int
}

// input fetch failure keeps counting in that stage

func NewSearchResults(resource string, filter string, value string, a *client.App, v View) View {
	list := client.NewList()
	list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	// move to app
	w, h := ui.TerminalDimensions()
	list.SetRect(0, 0, w, h)
	return &searchResults{
		v:         v,
		a:         a,
		list:      list,
		resource:  resource,
		name:      fmt.Sprintf("%s/%s", a.Instance, resource),
		filter:    fmt.Sprintf("%s=%s", filter, value),
		pageToken: "",
	}
}

type opRow struct {
	s *searchResults
	o opInt
	m *reapi.RequestMetadata
}

type opInt interface {
	name() string
	label(r opRow) string
}

type op struct {
	o *longrunning.Operation
}

func (op op) name() string {
	return op.o.Name
}

func (o op) label(r opRow) string {
	return o.name()
}

func newOp(o *longrunning.Operation) *op {
	return &op{o: o}
}

type ci struct {
	op
	n     int
	start time.Time
	done  time.Time
}

func (ci ci) label(r opRow) string {
	label := ci.op.label(r)
	if ci.n > 0 {
		label += fmt.Sprintf(" (%d)", ci.n)
	}
	return label
}

func newCI(o *longrunning.Operation) *ci {
	return &ci{
		op: *newOp(o),
	}
}

func newTI(o *longrunning.Operation) *ci {
	return &ci{
		op: *newOp(o),
	}
}

type ex struct {
	op
	m *reapi.RequestMetadata
	e *reapi.ExecutedActionMetadata

	start     time.Time
	done      time.Time
	fetch     time.Duration
	execution time.Duration
	wall      time.Duration
}

func (e ex) label(r opRow) string {
	return strings.TrimPrefix(e.name(), fmt.Sprintf("%s/executions/", r.s.a.Instance))
}

func newEx(o *longrunning.Operation) *ex {
	e, err := client.ExecutedActionMetadata(o)
	if err != nil {
		panic(err)
	}
	return &ex{
		op: *newOp(o),
		m:  client.RequestMetadata(o),
		e:  e,
	}
}

func (r opRow) String() string {
	// mode stuff
	if r.s.mode == 0 && r.m != nil {
		return r.m.TargetId
	}
	return r.o.label(r)
}

func (s *searchResults) fetch() []*longrunning.Operation {
	c := longrunning.NewOperationsClient(s.a.Conn)
	request := &longrunning.ListOperationsRequest{
		Name:      s.name,
		Filter:    s.filter,
		PageSize:  100,
		PageToken: s.pageToken,
	}
	r, err := c.ListOperations(context.Background(), request)
	if err != nil {
		ui.Close()
		fmt.Printf("%s\n", request.String())
		panic(err)
	}
	s.pageToken = r.NextPageToken
	if s.pageToken == "" {
		s.fetched = true
		s.fetchDetailsIndex = 0
	}
	return r.Operations
}

func (s *searchResults) updateSelectedName() {
	if len(s.list.Rows) > s.list.SelectedRow {
		s.selectedName = s.list.Rows[s.list.SelectedRow].(*opRow).o.name()
	}
}

type by_exec func(o1, o2 opRow) bool

type executionSorter struct {
	executions []*opRow
	by         func(w1, w2 opRow) bool // Closure used in the Less method.
}

func (s *executionSorter) Len() int {
	return len(s.executions)
}

func (s *executionSorter) Swap(i, j int) {
	s.executions[i], s.executions[j] = s.executions[j], s.executions[i]
}

func (s *executionSorter) Less(i, j int) bool {
	return !s.by(*s.executions[i], *s.executions[j])
}

func (by by_exec) Sort(executions []*opRow) {
	es := &executionSorter{
		executions: executions,
		by:         by, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(es)
}

func (s *searchResults) fetchExecutionDetail() {
	exec := func(o1, o2 opRow) bool {
		ex1, ex2 := o1.o.(*ex), o2.o.(*ex)
		if ex1.o.Done == ex2.o.Done && ex1.m != nil && ex2.m != nil {
			return ex1.m.TargetId > ex2.m.TargetId
		}
		return ex1.o.Done && !ex2.o.Done
	}
	by_exec(exec).Sort(s.opRows)
	for _, r := range s.opRows {
		s.list.Rows = append(s.list.Rows, r)
	}
	s.updateSelectedName()
	s.fetchDetailsIndex = -1
}

func (s *searchResults) fetchCorrelatedInvocationsDetail() {
	for _, r := range s.opRows {
		s.list.Rows = append(s.list.Rows, r)
	}
	s.updateSelectedName()
	s.fetchDetailsIndex = -1
}

func (s *searchResults) fetchDetails() {
	switch s.resource {
	case "executions":
		s.fetchExecutionDetail()
	case "toolInvocations":
		s.fetchToolInvocationDetail()
	case "correlatedInvocations":
		s.fetchCorrelatedInvocationsDetail()
	}
}

func (s *searchResults) fetchToolInvocationDetail() {
	opRow := s.opRows[s.fetchDetailsIndex]
	name := opRow.o.name()
	c := longrunning.NewOperationsClient(s.a.Conn)
	r, err := c.ListOperations(context.Background(), &longrunning.ListOperationsRequest{
		Name:      fmt.Sprintf("%s/executions", s.a.Instance),
		Filter:    fmt.Sprintf("toolInvocationId=%s", name),
		PageSize:  100,
		PageToken: s.pageToken,
	})
	if err != nil {
		panic(err)
	}
	s.pageToken = r.NextPageToken
	if s.pageToken == "" {
		s.fetchDetailsIndex++
		if s.fetchDetailsIndex >= len(s.list.Rows) {
			s.fetchDetailsIndex = -1
		}
	}
	n := len(r.Operations)
	ci := opRow.o.(*ci)
	if ci.n == 0 && n != 0 {
		s.list.Rows = append(s.list.Rows, opRow)
	}
	ci.n += n
	s.updateSelectedName()

	// ops := Map[*longrunning.Operation, op](r.Operations, newEx)
	// opRow.start = max(Map(ops, func (o op) { return o.start }))
	// opRow.done = max(Map(ops, func (o op) { return o.done }))
	// s.detailOps = r.Operations
}

func (s *searchResults) updateTitle() {
	s.list.Title = fmt.Sprintf("%s - %s (%d)", s.name, s.filter, len(s.list.Rows))
}

func (s *searchResults) Update() {
	if !s.fetched {
		s.a.Fetches++
		ops := s.fetch()

		for _, o := range ops {
			var op opInt
			switch s.resource {
			case "correlatedInvocations":
				op = newCI(o)
			case "toolInvocations":
				op = newTI(o)
			case "executions":
				op = newEx(o)
			}

			s.opRows = append(s.opRows, &opRow{s: s, o: op, m: client.RequestMetadata(o)})
		}
	} else if s.fetchDetailsIndex != -1 {
		s.a.Fetches++
		s.fetchDetails()
		s.updateTitle()
	}
}

func (s *searchResults) Handle(e ui.Event) View {
	switch e.ID {
	case "q", "<Escape>":
		return s.v
	case "j", "<Down>":
		s.list.ScrollDown()
		s.updateSelectedName()
	case "k", "<Up>":
		s.list.ScrollUp()
		s.updateSelectedName()
	case "<PageDown>":
		s.list.ScrollAmount(s.list.Inner.Dy())
		s.updateSelectedName()
	case "<PageUp>":
		s.list.ScrollAmount(-s.list.Inner.Dy())
		s.updateSelectedName()
	case "<Enter>":
		// could be nicer and just send the op
		if s.resource == "executions" {
			return NewDocument(s.a, s.selectedName, s)
		} else if s.resource == "toolInvocations" {
			return NewSearchResults("executions", "toolInvocationId", s.selectedName, s.a, s)
		} else if s.resource == "correlatedInvocations" {
			return NewSearchResults("toolInvocations", "correlatedInvocationsId", s.selectedName, s.a, s)
		}
	}
	return s
}

func (s searchResults) Render() []ui.Drawable {
	return []ui.Drawable{s.list}
}
