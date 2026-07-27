package view

import (
	"os"
	// "google.golang.org/protobuf/encoding/prototext"
	"container/list"
	"context"
	"fmt"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/buildfarm/bf-client/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"maps"
	"slices"
	"sort"
	"strings"
	"strconv"
	"sync"
	"time"
)

var workersSorts = []string{"Executions", "Name"}
var workersViews = []string{"Slots", "Actions"}

type profileResult struct {
	name    string
	profile *bfpb.WorkerProfileMessage
	stale   int
	message string
}

type stats struct {
	workers        []string
	status         bfpb.BackplaneStatus
	profiles       map[string]*profileResult
	last           time.Time
	prequeueData   list.List
	prequeueSum    float64
	queueData      list.List
	queueSum       float64
	dispatchedData list.List
	dispatchedSum  float64
	ticks          float64
	mutex          *sync.Mutex
}

type numValue struct {
	fmt    string
	value  int
	mode   int
	parent *numValue
}

func (nv numValue) String(selected bool) string {
	return fmt.Sprintf(nv.fmt, nv.value)
}

type Queue struct {
	c            client.Component
	focused      bool
	s            stats
	meter        *client.List
	stats        *client.Tree
	workers      numValue
	prequeueNode client.TreeNode
	prequeue     numValue
	queueNode    client.TreeNode
	queue        numValue
	dispatched   numValue
	workersSort  int
	workersView  int
	settings     *settings
}

func statNode(nv *numValue) *client.TreeNode {
	return &client.TreeNode{Value: nv}
}

type workersTitle struct {
	q *Queue
}

func (s workersTitle) String() string {
	return fmt.Sprintf(">%s< %s", workersSorts[s.q.workersSort], workersViews[s.q.workersView])
}

func NewQueue(c client.Component, selected int) *Queue {
	meter := client.NewList()
	meter.SelectedRow = -1
	q := &Queue{
		c: c,
		s: stats{
			profiles: make(map[string]*profileResult),
			last:     time.Now(),
			mutex:    &sync.Mutex{},
		},
		meter:       meter,
		stats:       client.NewTree(),
		workers:     numValue{fmt: "Workers: %v"},
		prequeue:    numValue{fmt: "Prequeue: %v", mode: 1},
		queue:       numValue{fmt: "Queue: %v", mode: 2},
		dispatched:  numValue{fmt: "Dispatched: %v", mode: 3},
		workersSort: 0,
	}
	meter.SubTitle = &workersTitle{q: q}
	q.stats.Focused = true
	q.stats.SelectedRow = selected
	q.stats.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	q.queueNode.Value = &q.queue
	q.prequeueNode.Value = &q.prequeue
	q.stats.SetNodes([]*client.TreeNode{
		statNode(&q.workers),
		&q.prequeueNode,
		&q.queueNode,
		statNode(&q.dispatched),
	})
	return q
}

func getNumber(title string, v View, action func(int) bool) View {
	return NewTextEntry(title, func(s string) bool {
		n, err := strconv.Atoi(s)
		if err != nil {
			return false
		}

		return action(n)
	}, UISize(), v)
}

func (v *Queue) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		v.c.Finish()
	case "J", "<PageDown>":
		if v.meter.SelectedRow != -1 {
			v.meter.ScrollAmount(v.meter.Inner.Dy())
		}
	case "K", "<PageUp>":
		if v.meter.SelectedRow != -1 {
			v.meter.ScrollAmount(-v.meter.Inner.Dy())
		}
	case "j", "<Down>":
		if v.stats.Focused {
			v.stats.ScrollDown()
		}
		if v.meter.SelectedRow != -1 {
			v.meter.ScrollDown()
		}
	case "k", "<Up>":
		if v.stats.Focused {
			v.stats.ScrollUp()
		} else {
			v.meter.ScrollUp()
		}
	case "E":
		if !v.stats.Focused {
			return getNumber("Execute Width", v, func(width int) bool {
				if (width < 0) {
					return false
				}
				for _, worker := range v.s.workers {
					w := NewWorker(v.c, worker, v)
					w.changeWidth(int32(width))
				}
				return true
			})
		}
	case "h", "<Left>":
		if v.stats.Focused {
			v.stats.Collapse()
			ui.Clear()
		} else {
			v.stats.Focused = v.meter.SelectedRow != -1
			if v.meter.SelectedRow == -1 {
				v.meter.SelectedRow = 0
			} else {
				v.meter.SelectedRow = -1
			}
		}
	case "s":
		return newSettings(v.c.App(), v)
	case "H", "<S-Left>":
		n := v.stats.SelectedNode()
		for _, cn := range n.Nodes {
			expandNodeAll(cn, false)
		}
		v.stats.Collapse()
	case "l", "<Right>":
		if v.stats.SelectedRow != 0 {
			if v.stats.Focused {
				v.stats.Expand()
				ui.Clear()
			}
		} else if v.meter.SelectedRow == -1 && len(v.meter.Rows) > 0 {
			v.stats.Focused = v.meter.SelectedRow != -1
			if v.meter.SelectedRow == -1 {
				v.meter.SelectedRow = 0
			} else {
				v.meter.SelectedRow = -1
			}
		}
	case "L", "<S-Right>":
		n := v.stats.SelectedNode()
		for _, cn := range n.Nodes {
			expandNodeAll(cn, true)
		}
		v.stats.Expand()
	case "<Enter>":
		if v.meter.SelectedRow >= 0 {
			// get the worker out of the list
			if v.meter.SelectedRow < len(v.meter.Rows) {
				return NewWorker(v.c, v.meter.Rows[v.meter.SelectedRow].(Worker).w, v)
			}
		} else if v.stats.SelectedNode().Value.(*numValue).mode != 0 {
			ui.Clear()
			return NewOperationList(v.c, v.stats.SelectedNode().Value.(*numValue).mode, v)
		}
	case "e":
		return NewExecution(v.c, "test", v)
	case "D":
		return NewDiffMenu(v.c, os.Getenv("HOME") + "/.cache/bf-client/diff", v)
	case "/":
		return NewSearch(v.c, v)
	case "T":
		return NewTest(v.c.App(), v)
	case "<Tab>":
		if v.stats.SelectedRow == 0 {
			v.workersView++
			v.workersView %= len(workersViews)
		}
	case ">":
		if v.stats.SelectedRow == 0 {
			v.workersSort++
			v.workersSort %= len(workersSorts)
		}
	case "<":
		if v.stats.SelectedRow == 0 {
			v.workersSort += len(workersSorts) - 1
			v.workersSort %= len(workersSorts)
		}
		/*
		  case "S":
		    return NewServerTest(v.c.App(), v)
		*/
	}
	return v
}

func updateProvisionNode(node *client.TreeNode, provision *bfpb.QueueStatus) {
	size := int64(0)
	n := len(provision.InternalSizes)
	if len(node.Nodes) != n {
		node.Nodes = make([]*client.TreeNode, n)
		for i := 0; i < n; i++ {
			node.Nodes[i] = statNode(&numValue{fmt: "%v", parent: node.Value.(*numValue)})
		}
	}
	for i, of := range provision.InternalSizes {
		nodeNumValue := node.Nodes[i].Value.(*numValue)
		nodeNumValue.value = int(of)
		size += of
	}
	nodeNumValue := node.Value.(*numValue)
	nodeNumValue.value = int(size)
}

func (v *Queue) updateProvisionNodes(provisions []*bfpb.QueueStatus) {
	if len(v.queueNode.Nodes) != len(provisions) {
		for _, provision := range provisions {
			node := &client.TreeNode{
				Value: &numValue{fmt: provision.Name + ": %v", parent: &v.queue},
				Nodes: make([]*client.TreeNode, 0),
			}
			v.queueNode.Nodes = append(v.queueNode.Nodes, node)
		}
	}
	for i, provision := range provisions {
		updateProvisionNode(v.queueNode.Nodes[i], provision)
	}
}

func (v *Queue) Update() {
	s := &v.s
	c := bfpb.NewOperationQueueClient(v.c.App().Conn)
	var st *bfpb.BackplaneStatus
	if v.stats.SelectedRow == 0 {
		if s.workers != nil {
			var wg sync.WaitGroup
			for _, worker := range s.workers {
				wg.Add(1)
				go fetchProfile(v, worker, v.c.App().GetWorkerConn(worker, v.c.App().CA), &wg)
			}
			wg.Wait()
		}
	}
	start := time.Now()
	st, err := c.Status(context.Background(), &bfpb.BackplaneStatusRequest {
		InstanceName: "shard",
	})
	v.c.App().LastReapiLatency = time.Since(start)
	if err == nil {
		s.status = *st
		s.workers = st.ActiveExecuteWorkers;
		// delete all members of s.profiles that aren't in s.workers
		activeWorkers := map[string]bool{}
		for _, worker := range s.workers {
			activeWorkers[worker] = true
		}
		inactiveWorkers := []string{}
		for name := range s.profiles {
			if !activeWorkers[name] {
				inactiveWorkers = append(inactiveWorkers, name)
		  }
		}
		for _, name := range inactiveWorkers {
			delete(s.profiles, name)
	  }
	} else {
		panic(err)
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.Unknown && st.Code() != codes.Unavailable) {
			panic(err)
		}
		return
	}
	v.workers.value = len(s.workers)
	v.prequeue.value = int(s.status.Prequeue.Size)
	v.queue.value = int(s.status.OperationQueue.Size)
	v.dispatched.value = int(s.status.DispatchedSize)
	now := time.Now()
	if s.last.Add(time.Second / 10).Before(now) {
		if s.prequeueData.Len() > 60 {
			s.prequeueData.Remove(s.prequeueData.Back())
			s.queueData.Remove(s.queueData.Back())
			s.dispatchedData.Remove(s.dispatchedData.Back())
		}
		if s.ticks > 0 {
			s.prequeueData.PushFront(s.prequeueSum / s.ticks)
			s.queueData.PushFront(s.queueSum / s.ticks)
			s.dispatchedData.PushFront(s.dispatchedSum / s.ticks)
		} else {
			s.prequeueData.PushFront(float64(0));
			s.queueData.PushFront(float64(0));
			s.dispatchedData.PushFront(float64(0));
		}
		s.prequeueSum = 0
		s.queueSum = 0
		s.dispatchedSum = 0
		s.ticks = 0
		s.last = now
	}
	if st != nil {
		s.prequeueSum += float64(s.status.Prequeue.Size)
		s.queueSum = float64(s.status.OperationQueue.Size)
		s.dispatchedSum += float64(s.status.DispatchedSize)
	}
	updateProvisionNode(&v.prequeueNode, s.status.Prequeue)
	v.updateProvisionNodes(s.status.OperationQueue.Provisions)
	s.ticks++
}

type dims struct {
	width  int
	height int
}

func nodeDimensions(node *client.TreeNode, level int) dims {
	d := dims{len(node.Value.String(false)) + level*2, 1}
	if node.Expanded {
		for _, n := range node.Nodes {
			nd := nodeDimensions(n, level+1)
			if nd.width > d.width {
				d.width = nd.width
			}
			d.height += nd.height
		}
	}
	return d
}

func treeDimensions(t *client.Tree) dims {
	d := dims{0, 0}
	t.Walk(func(n *client.TreeNode) int {
		nd := nodeDimensions(n, 0)
		if nd.width > d.width {
			d.width = nd.width
		}
		d.height += nd.height
		return -1
	})
	return d
}

func (v Queue) Render() []ui.Drawable {
	s := v.s
	p := widgets.NewParagraph()
	p.Text = fmt.Sprintf("%v: %v\n%v", v.c.App().ReapiHost, v.c.App().LastReapiLatency, formatTime(s.last))
	p.SetRect(0, 0, 80, 4)

	d := treeDimensions(v.stats)
	d.width += 4
	d.height += 3
	v.stats.SetRect(0, 3, d.width, d.height)

	var info ui.Drawable
	if v.stats.SelectedRow == 0 {
		info = renderWorkersInfo(&s, v.meter, d.width, v.c.Dimensions().Height, v.workersSort, v.workersView)
	} else {
		plot := widgets.NewPlot()
		plot.Data = make([][]float64, 1)
		var chartValue *numValue
		for chartValue, _ = v.stats.SelectedNode().Value.(*numValue); chartValue.parent != nil; chartValue = chartValue.parent {
		}
		mode := chartValue.mode
		var container list.List
		switch mode {
		case 1:
			plot.Title = "Prequeue"
			plot.LineColors[0] = ui.ColorRed
			container = s.prequeueData
		case 2:
			plot.Title = "Queue"
			plot.LineColors[0] = ui.ColorYellow
			container = s.queueData
		case 3:
			plot.Title = "Dispatched"
			plot.LineColors[0] = ui.ColorCyan
			container = s.dispatchedData
		}
		plot.Data[0] = make([]float64, 60)
		n := 0
		for e := container.Front(); n < 60 && n < container.Len(); n++ {
			plot.Data[0][59-n] = e.Value.(float64)
			e = e.Next()
		}
		for ; n < 60; n++ {
			plot.Data[0][59-n] = float64(0)
		}
		plot.SetRect(d.width, 3, d.width+71, 30)
		plot.AxesColor = ui.ColorWhite
		plot.Marker = widgets.MarkerBraille
		plot.PlotType = widgets.ScatterPlot

		info = plot
	}

	return []ui.Drawable{p, v.stats, info}
}

func fetchProfile(v *Queue, worker string, conn *grpc.ClientConn, wg *sync.WaitGroup) {
	defer wg.Done()

	workerProfile := bfpb.NewWorkerProfileClient(conn)
	clientDeadline := time.Now().Add(time.Millisecond * 30)
	ctx, _ := context.WithDeadline(context.Background(), clientDeadline)
	profile, err := workerProfile.GetWorkerProfile(ctx, &bfpb.WorkerProfileRequest{})
	if err == nil {
		v.s.mutex.Lock()
		v.s.profiles[worker] = &profileResult{name: worker, profile: profile, message: ""}
		v.s.mutex.Unlock()
	} else {
		st, ok := status.FromError(err)
		v.s.mutex.Lock()
		if !ok || st.Code() != codes.DeadlineExceeded {
			result := v.s.profiles[worker]
			if result == nil {
				v.s.profiles[worker] = &profileResult{name: worker, profile: &bfpb.WorkerProfileMessage{}, stale: 1, message: st.String()}
			} else {
				result.stale++
				result.message = st.String()
			}
		} else {
			result := v.s.profiles[worker]
			if result == nil {
				v.s.profiles[worker] = &profileResult{name: worker, profile: &bfpb.WorkerProfileMessage{}, stale: 1, message: ""}
			} else {
				result.stale++
			}
		}
		v.s.mutex.Unlock()
	}
}

type byProfile func(w1, w2 *profileResult) bool

func (by byProfile) Sort(workers []*profileResult) {
	ws := &workerSorter{
		workers: workers,
		by:      by, // The Sort method's receiver is the function (closure) that defines the sort order.
	}
	sort.Sort(ws)
}

type workerSorter struct {
	workers []*profileResult
	by      func(w1, w2 *profileResult) bool // Closure used in the Less method.
}

// Len is part of sort.Interface.
func (s *workerSorter) Len() int {
	return len(s.workers)
}

// Swap is part of sort.Interface.
func (s *workerSorter) Swap(i, j int) {
	s.workers[i], s.workers[j] = s.workers[j], s.workers[i]
}

// Less is part of sort.Interface. It is implemented by calling the "by" closure in the sorter.
func (s *workerSorter) Less(i, j int) bool {
	return !s.by(s.workers[i], s.workers[j])
}

type Worker struct {
	w   string
	row string
}

func (w Worker) String() string {
	return w.row
}

func sortWorkers(profiles []*profileResult, sort int) []*profileResult {
	exec_used := func(profileResult *profileResult) int {
		if profileResult == nil {
			return -1
		}
		profile := profileResult.profile
		for _, stage := range profile.Stages {
			if stage.Name == "ExecuteActionStage" {
				return int(stage.SlotsUsed)
			}
		}
		return 0
	}
	exec_avail := func(profileResult *profileResult) int {
		if profileResult == nil {
			return -1
		}
		profile := profileResult.profile
		for _, stage := range profile.Stages {
			if stage.Name == "ExecuteActionStage" {
				return int(stage.SlotsConfigured)
			}
		}
		return 0
	}
	exec := func(w1, w2 *profileResult) bool {
		if exec_used(w1) == exec_used(w2) {
			if exec_avail(w1) == exec_avail(w2) {
				return w1.name < w2.name
			}
			return exec_avail(w1) < exec_avail(w2)
		}
		return exec_used(w1) < exec_used(w2)
	}
	name := func(w1, w2 *profileResult) bool {
		w1name, w2name := w1.name, w2.name
		if len(w1.profile.Name) != 0 {
			w1name = w1.profile.Name
		}
		if len(w2.profile.Name) != 0 {
			w2name = w2.profile.Name
		}
		return w1name > w2name
	}
	if sort == 0 {
		byProfile(exec).Sort(profiles)
	} else {
		byProfile(name).Sort(profiles)
	}

	return profiles
}

// List needs work on draw, flip for only background, etc
func renderWorkersInfo(s *stats, meter *client.List, x int, h int, sort int, view int) ui.Drawable {
	meter.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	height := Min(len(s.profiles), h-6)
	width := 159
	// width = 100
	meter.SetRect(x, 3, x+width+2, 4+height+2)
	meter.Title = "Workers"

	wl := 0
	for _, profile := range s.profiles {
		twl := len(profile.profile.Name)
		if twl > wl {
			wl = twl
		}
	}

	profiles := sortWorkers(slices.Collect(maps.Values(s.profiles)), sort)

	n := 0
	plen := len(profiles)
	rows := make([]fmt.Stringer, plen)
	for _, p := range profiles {
		rows[n] = renderWorkerRow(p, wl, width - wl, view)
		n++
	}
	meter.Rows = rows

	return meter
}

func countBar(used int, slots int) string {
	// # used/slots #
	if slots == 0 {
		return fmt.Sprintf("# %d #", used)
	}
	var format_string string
	if slots < 10 {
		format_string = "# %d/%d #"
	} else if slots < 100 {
		format_string = "# %2d/%2d #"
	} else {
		format_string = "# %3d/%3d #"
	}
	return fmt.Sprintf(format_string, used, slots)
}

func stageAttr(color string, reverse bool) string {
	attr := "fg:"
	if reverse {
		attr += "black,mod:dim,bg:"
	}
	return attr + color
}

func pad(s string, l int) string {
	return strings.Repeat(" ", l - len(s)) + s
}

/* needs so much more */
/* expand direction, reasonable width over entire worker */
func renderStage(color string, used int32, capacity int32, right bool, vw *int) string {
	c := len(fmt.Sprintf("%d", capacity))
	*vw = c * 2 + 5
	return renderStageExpand(color, int(used), int(capacity), right, c, *vw)
}

func renderStageExpand(color string, used int, capacity int, right bool, c int, w int) string {
	text := ""
	if used >= w {
		if capacity == 0 {
			text = fmt.Sprintf(" %d ", used)
		} else {
			u := fmt.Sprintf("%d", used)
			text = fmt.Sprintf(" %s/%d ", pad(u, c), capacity)
		}
		p := w - len(text)
		// hopefully p > 0
		text = strings.Repeat("#", (p + 1) / 2) + text + strings.Repeat("#", (p / 2))
	} else {
		text = strings.Repeat("#", used)
		if right {
			text = pad(text, w)
		} else {
			text += strings.Repeat(" ", w - used)
		}
	}
	attr := stageAttr(color, used >= capacity)
	return fmt.Sprintf("[%s](%s)", text, attr)
}

func getStage(profile *bfpb.WorkerProfileMessage, name string) *bfpb.StageInformation {
	for _, stage := range profile.Stages {
		if stage.Name == name {
			return stage
		}
	}
	return nil
}

func renderWorkerRow(r *profileResult, wl int, gl int, view int) Worker {
	var profile *bfpb.WorkerProfileMessage
	if r == nil {
		r = &profileResult{profile: &bfpb.WorkerProfileMessage{}, stale: 1, message: "uninitialized"}
	}
	profile = r.profile

	type column struct {
		name string
		expand bool
		color string
		stage bool
		right bool
	}
	execColor := ""
	if view == 1 {
		execColor = "yellow"
	} else {
		execColor = "red"
	}
	columns := []column {
		column { name: "Worker Name" },
		column { name: "sep" },
		column { name: "InputFetchStage", color: "blue", right: true, stage: true },
		column { name: "ExecuteActionStage", color: execColor, stage: true, expand: true },
		column { name: "ReportResultStage", color: "green", stage: true },
		column { name: "stale" },
	}
	data := map[string]string {}

	// get wl out of here
	data["Worker Name"] = pad(profile.Name, wl)
	sep := ": "
	gl -= len(sep)
	data["sep"] = sep
	stale := "      "
	if r.stale != 0 {
		stale = " stale"
	}
	gl -= len(stale)
	data["stale"] = stale

	expansions := 0
	for _, column := range columns {
		if column.expand {
			expansions++
		} else if column.stage {
			stage := getStage(profile, column.name)

			if stage != nil {
				var vw int
				data[column.name] = renderStage(column.color, stage.SlotsUsed, stage.SlotsConfigured, column.right, &vw)
				gl -= vw
			} else {
				data[column.name] = " NO DATA "
			}
		}
	}

	ew := gl / expansions

	row := ""
	for _, column := range columns {
		if column.stage && column.expand {
			stage := getStage(profile, column.name)

			if stage != nil {
				used, capacity := int(stage.SlotsUsed), int(stage.SlotsConfigured)
				if view == 1 && column.name == "ExecuteActionStage" {
					used, capacity = len(stage.OperationNames) + len(stage.Executions), 0
				}
				row += renderStageExpand(column.color, used, capacity, false, len(fmt.Sprintf("%d", capacity)), ew)
			} else {
				row += pad(" NO DATA ", ew)
			}
		} else {
			row += data[column.name]
		}
	}

	return Worker{
		w:   r.name,
		row: row,
	}
}

func xrenderWorkerRow(r *profileResult, wl int, gl int, view int) Worker {
	var profile *bfpb.WorkerProfileMessage
	if r == nil {
		r = &profileResult{profile: &bfpb.WorkerProfileMessage{}, stale: 1, message: "uninitialized"}
	}
	profile = r.profile
	var input_fetch_used, input_fetch_slots int
	var execute_action_used, execute_action_slots int
	var report_result_used, report_result_slots int
	for _, stage := range profile.Stages {
		if stage.Name == "InputFetchStage" {
			input_fetch_used = int(stage.SlotsUsed)
			input_fetch_slots = int(stage.SlotsConfigured)
		} else if stage.Name == "ExecuteActionStage" {
			if view == 0 {
				execute_action_used = int(stage.SlotsUsed)
				execute_action_slots = int(stage.SlotsConfigured)
			} else {
				execute_action_used = len(stage.OperationNames) + len(stage.Executions)
				execute_action_slots = 0
			}
		} else if stage.Name == "ReportResultStage" {
			report_result_used = int(stage.SlotsUsed)
			report_result_slots = int(stage.SlotsConfigured)
		}
	}
	name := r.name
	if len(profile.Name) > 0 {
		name = profile.Name
	}
	row := ""
	if wl > len(name) {
		row += strings.Repeat(" ", wl-len(name))
	}
	row += name + ": ["
	fetchCountBar := countBar(input_fetch_used, input_fetch_slots)
	maxWidth := len(countBar(input_fetch_slots, input_fetch_slots))
	if input_fetch_used > maxWidth {
		row += strings.Repeat(" ", maxWidth-len(fetchCountBar))
		row += fetchCountBar + "]("
	} else {
		width := Min(input_fetch_slots, maxWidth)
		row += strings.Repeat(" ", width-input_fetch_used)
		row += strings.Repeat("#", input_fetch_used) + "]("
	}
	if input_fetch_used == input_fetch_slots {
		row += "fg:black,mod:dim,bg:blue"
	} else {
		row += "fg:blue"
	}
	// do some math on the rest of this so that it fits nicely in the display
	row += ")["
	// need to consider size of screen and skip the closing bar if we're over
	executeCount := false
	executeCountBar := countBar(execute_action_used, execute_action_slots)
	if execute_action_used > len(executeCountBar) {
		row += executeCountBar + "]("
		executeCount = true
	} else {
		width := Min(execute_action_slots, len(executeCountBar))
		row += strings.Repeat("#", execute_action_used)
		padding := width - execute_action_used
		if padding > 0 {
			row += strings.Repeat(" ", padding)
		}
		row += "]("
	}
	execute_color := "red"
	if view == 1 {
		execute_color = "yellow"
	}
	if execute_action_used == execute_action_slots {
		row += "fg:black,mod:dim,bg:" + execute_color
		panic(row)
	} else {
		row += "fg:" + execute_color
	}
	row += ")["
	row += strings.Repeat("#", report_result_used)
	if report_result_slots < report_result_used {
		panic(fmt.Sprintf("HOW?? %d %d", report_result_slots, report_result_used))
	}
	row += strings.Repeat(" ", report_result_slots-report_result_used) + "]("
	if report_result_used == report_result_slots {
		row += "fg:black,mod:dim,bg:green"
	} else {
		row += "fg:green"
	}
	row += ")"
	if !executeCount {
		row += fmt.Sprintf(" %d/%d", execute_action_used, execute_action_slots)
	}
	if r.stale > 0 {
		row += " stale"
		if len(r.message) > 0 {
			row += ", " + r.message
		}
	}
	return Worker{
		w:   r.name,
		row: row,
	}
}

func formatTime(t time.Time) string {
	return t.Format("Mon Jan 2 15:04:05 -0700 MST 2006")
}
