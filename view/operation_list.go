package view

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	ui "github.com/gizak/termui/v3"
	"github.com/golang/protobuf/ptypes"
	"github.com/hashicorp/golang-lru/v2"
	"github.com/werkt/bf-client/client"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type operation struct {
	target          string
	mnemonic        string
	build           string
	workerStart     *time.Time
	workerCompleted *time.Time
	done            bool
}

type operationList struct {
	list       *client.List
	Filter     string
	Select     map[string]string
	Name       string
	a          *client.App
	opNames    []string
	ops        []*longrunning.Operation
	prevNames  map[string]*longrunning.Operation
	opcache    *lru.Cache[string, operation]
	mode       int
	v          View
	field      int
	reversed   bool
	queues     []*client.Queue
	fetchToken string
	grouped    bool

	fetchChanged bool
	stall        int
	stallStart   int
	debug        bool
}

func NewOperationList(a *client.App, mode int, v View) *operationList {
	opcache, _ := lru.New[string, operation](10000)
	var queues []*client.Queue
	if mode != 3 {
		c := bfpb.NewOperationQueueClient(a.Conn)
		start := time.Now()
		a.Fetches++
		status, err := c.Status(context.Background(), &bfpb.BackplaneStatusRequest{
			InstanceName: "shard",
		})
		a.LastReapiLatency = time.Since(start)
		if err != nil {
			panic(err)
		}
		var names []string
		if mode == 1 {
			names = append(names, status.Prequeue.Name)
		} else {
			for _, queue := range status.OperationQueue.Provisions {
				names = append(names, queue.Name)
			}
		}
		for _, name := range names {
			queues = append(queues, client.NewQueue(context.Background(), a.Client, name))
		}
	}
	return &operationList{
		Name:       "executions",
		list:       client.NewList(),
		a:          a,
		mode:       mode,
		opcache:    opcache,
		field:      0,
		reversed:   false,
		v:          v,
		queues:     queues,
		fetchToken: "",
		prevNames:  make(map[string]*longrunning.Operation),
	}
}

func (v operationList) fetchQueues(max int64, cb func(string) (*client.Operation, error)) []*client.Operation {
	var ops []*client.Operation
	for _, queue := range v.queues {
		ops = append(ops, queue.Slice(context.Background(), v.a.Client, 0, max, cb)...)
		if int64(len(ops)) >= max {
			break
		}
	}
	return ops
}

func (v *operationList) fetchIteration(c longrunning.OperationsClient) {
	var retry int
	var retryErr error = nil
	var r *longrunning.ListOperationsResponse = nil
	// TODO put retries into next iteration cycle
	for retry = 5; r == nil && retry > 0; retry-- {
		req := &longrunning.ListOperationsRequest{
			Name:      fmt.Sprintf("%s/%s", v.a.Instance, v.Name),
			Filter:    v.Filter,
			PageSize:  100,
			PageToken: v.fetchToken,
		}
		var err error
		v.a.Fetches++
		r, err = c.ListOperations(context.Background(), req)
		if err != nil {
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.Unavailable {
				panic(err)
			} else {
				retryErr = err
				retry--
				r = nil
			}
		}
	}
	if retry == 0 {
		panic(retryErr)
	}
	var page []*longrunning.Operation
	for _, op := range r.Operations {
		v.ops = append(v.ops, op)
		page = append(page, op)
		m := client.RequestMetadata(op)
		if m != nil {
			addOpcache(v.a, v.opcache, op, m)
		}
	}
	v.fetchToken = r.NextPageToken
	countAdds := v.fetchToken != ""
	if !countAdds {
		page, v.ops = v.ops, make([]*longrunning.Operation, 0)
		// do timeout calculation here
		v.fetchChanged = v.fetchChanged || len(page) != len(v.prevNames)
		v.prevNames = make(map[string]*longrunning.Operation)

		// probably push into return
		if v.fetchChanged {
			v.stall = 0
			v.stallStart = 1
			v.fetchChanged = false
		} else {
			v.stall = v.stallStart
			if v.stallStart < 256 {
				v.stallStart <<= 1
			}
		}
	}
	for _, op := range page {
		_, exists := v.prevNames[op.Name]
		if countAdds && !exists {
			v.fetchChanged = true
		}
		v.prevNames[op.Name] = op
	}
}

func (v *operationList) fetchFiltered(filter string, name string) []*client.Operation {
	// need to remove the fetches here unless the current timer is expired
	// advance the timer by 2 if the fetch did not return anything new
	if v.Filter != filter {
		v.Filter = filter
		v.fetchToken = ""
		v.Name = name
		v.opNames = make([]string, 0)
	}

	if v.stall == 0 {
		c := longrunning.NewOperationsClient(v.a.Conn)
		v.fetchIteration(c)
	} else {
		v.stall--
	}

	r := make([]*client.Operation, len(v.prevNames))
	i := 0
	for name, op := range v.prevNames {
		r[i] = &client.Operation{Name: name, Done: op.Done}
		i++
	}
	return r
}

func (v operationList) xfetchFiltered(filter string, name string) []*client.Operation {
	var ops []*client.Operation
	c := longrunning.NewOperationsClient(v.a.Conn)
	for nextPageToken := "initial"; nextPageToken != ""; v.fetchToken = nextPageToken {
		var retry int
		var retryErr error = nil
		var r *longrunning.ListOperationsResponse = nil
		for retry = 5; r == nil && retry > 0; retry-- {
			req := &longrunning.ListOperationsRequest{
				Name:      fmt.Sprintf("%s/%s", v.a.Instance, name),
				Filter:    filter,
				PageSize:  100,
				PageToken: v.fetchToken,
			}
			var err error
			v.a.Fetches++
			r, err = c.ListOperations(context.Background(), req)
			if err != nil {
				st, ok := status.FromError(err)
				if !ok || st.Code() != codes.Unavailable {
					panic(err)
				} else {
					retryErr = err
					retry--
					r = nil
				}
			}
		}
		if retry == 0 {
			panic(retryErr)
		}
		opsPage := r.Operations
		for _, op := range opsPage {
			ops = append(ops, &client.Operation{Name: op.Name})
		}
		nextPageToken = r.NextPageToken
	}
	return ops
}

func (v operationList) queuesLength() int64 {
	var sum int64 = 0
	for _, queue := range v.queues {
		l, err := queue.Length(context.Background(), v.a.Client)
		if err != nil {
			panic(err)
		}
		sum += int64(l)
	}
	return sum
}

func (v *operationList) fetch() []*client.Operation {
	switch v.mode {
	case 1:
		return v.fetchFiltered("status=prequeued", v.Name)
	case 2:
		return v.fetchFiltered("status=queued", v.Name)
	case 3:
		return v.fetchFiltered("status=dispatched", v.Name)
	case 4:
		return v.fetchFiltered(v.Filter, v.Name)
	default:
		return make([]*client.Operation, 0)
	}
}

func (v *operationList) createOperationView() View {
	if len(v.opNames) > 0 {
		return NewDocument(v.a, v.list.Rows[v.list.SelectedRow].(*stageEx).name, v)
	}
	return v
}

func (v *operationList) selectedName() string {
	return v.list.Rows[v.list.SelectedRow].(*stageEx).name
}

func selectField(field int, name string) map[string]string {
	key := []string{"id", "target", "mnemonic", "build"}[field]
	return map[string]string{key: name}
}

func (v *operationList) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		ui.Clear()
		return v.v
	case "D":
		v.debug = !v.debug
	case "G":
		// group the operations in the list by the current visible field and count them
		v.grouped = !v.grouped
	case "j", "<Down>":
		v.list.ScrollDown()
	case "k", "<Up>":
		v.list.ScrollUp()
	case "h", "<Left>":
		v.field += 3
		v.field %= 4
	case "l", "<Right>":
		v.field++
		v.field %= 4
	case "<Enter>":
		ui.Clear()
		if v.grouped {
			sv := NewOperationList(v.a, v.mode, v)
			sv.Filter = v.Filter
			sv.Select = selectField(v.field, v.list.Rows[v.list.SelectedRow].(*groupResult).name)
			return sv
		} else {
			if v.Name == "executions" {
				return v.createOperationView()
			}
			if v.Name == "toolInvocations" {
				olv := NewOperationList(v.a, 4, v)
				olv.Filter = "toolInvocationId=" + v.selectedName()
				return olv
			}
		}
		return v
	case ">", "<":
		v.reversed = !v.reversed
	}
	return v
}

func addOpcache(a *client.App, opcache *lru.Cache[string, operation], o *longrunning.Operation, m *reapi.RequestMetadata) {
	eam, err := client.ExecutedActionMetadata(o)
	var workerStart *time.Time
	var workerCompleted *time.Time
	if err == nil && eam.WorkerStartTimestamp != nil {
		start, err := ptypes.Timestamp(eam.WorkerStartTimestamp)
		if err != nil {
			panic(err)
		}
		workerStart = &start
		if o.Done {
			completed, err := ptypes.Timestamp(eam.WorkerCompletedTimestamp)
			if err != nil {
				panic(err)
			}
			workerCompleted = &completed
		}
	}
	opcache.Add(o.Name, operation{
		build:           m.CorrelatedInvocationsId,
		target:          m.TargetId,
		mnemonic:        m.ActionMnemonic,
		workerStart:     workerStart,
		workerCompleted: workerCompleted,
		done:            o.Done,
	})
	a.Metadatas[o.Name] = m
	var opInvocations []string
	var ok bool
	if opInvocations, ok = a.Invocations[m.ToolInvocationId]; !ok {
		opInvocations = make([]string, 1)
		a.Invocations[m.ToolInvocationId] = opInvocations
	}
	a.Invocations[m.ToolInvocationId] = append(opInvocations, o.Name)
}

func (v *operationList) Update() {
	v.a.Fetches++
	ops := v.fetch()

	var wg sync.WaitGroup
	for _, op := range ops {
		if v.opcache.Contains(op.Name) {
			continue
		}
		if o, ok := v.a.Ops[op.Name]; !ok || o == nil {
			m := op.Metadata
			if m == nil {
				v.a.Fetches++
				wg.Add(1)
				go getExecution(v.a, op.Name, v.a.Conn, &wg)
			}
		}
	}
	wg.Wait()
	v.opNames = make([]string, 0)
	for _, op := range ops {
		v.opNames = append(v.opNames, op.Name)
		o, ok := v.a.Ops[op.Name]
		if !v.opcache.Contains(op.Name) && ok && o != nil {
			m := client.RequestMetadata(o)
			if m != nil {
				addOpcache(v.a, v.opcache, o, m)
				delete(v.a.Ops, op.Name)
			}
		}
	}
}

func (v operationList) modeTitle() string {
	switch v.mode {
	case 1:
		return "Prequeue"
	case 2:
		return "Queue"
	case 3:
		return "Dispatched"
	case 4:
		return fmt.Sprintf("Filter(%s)", v.Filter)
	default:
		return "Unknown"
	}
}

func opStringer(name string, op *operation, field func() int) *stageEx {
	row := &stageEx{
		field: field,
		name:  name,
	}
	if op != nil {
		row.done = op.done
		if op.done {
			row.final = *op.workerCompleted
		}
		if op.workerStart != nil {
			row.fence = *op.workerStart
		} else {
			// little abuse of our indicator here
			row.stalled = !op.done
		}
		row.target, row.mnemonic, row.build = op.target, op.mnemonic, op.build
	} else {
		row.target, row.mnemonic, row.build = "unknown", "unknown", "unknown"
	}
	return row
}

func selectOp(sel map[string]string, name string, op operation) bool {
	for key, value := range sel {
		if key == "id" && value != name {
			return false
		}
		if key == "target" && value != op.target {
			return false
		}
		if key == "mnemonic" && value != op.mnemonic {
			return false
		}
		if key == "build" && value != op.build {
			return false
		}
	}
	return true
}

func (v operationList) renderItemized() []fmt.Stringer {
	rows := []fmt.Stringer{}
	for _, name := range v.opNames {
		op, ok := v.opcache.Get(name)
		if !selectOp(v.Select, name, op) {
			continue
		}
		o := &op
		if !ok {
			o = nil
		}
		rows = append(rows, opStringer(name, o, func() int { return v.field }))
	}
	return rows
}

type groupResult struct {
	name  string
	count int
}

func (r groupResult) String() string {
	return fmt.Sprintf("%s: %d", r.name, r.count)
}

func (v operationList) renderGrouped() []fmt.Stringer {
	buckets := make(map[string]*groupResult)
	for _, name := range v.opNames {
		op, ok := v.opcache.Get(name)
		if !selectOp(v.Select, name, op) {
			continue
		}
		o := &op
		if !ok {
			o = nil
		}
		val := opStringer(name, o, func() int { return v.field }).label()
		r, ok := buckets[val]
		if !ok {
			r = &groupResult{
				name:  val,
				count: 0,
			}
			buckets[val] = r
		}
		r.count++
	}

	results := slices.SortedFunc(maps.Values(buckets), func(a, b *groupResult) int {
		c := cmp.Compare(b.count, a.count)
		if c == 0 {
			c = cmp.Compare(b.name, a.name)
		}
		if v.reversed {
			c = -c
		}
		return c
	})
	rows := make([]fmt.Stringer, len(results))
	for i, r := range results {
		rows[i] = r
	}
	return rows
}

func fieldName(grouped bool, field int, sel map[string]string) string {
	fields := [...]string{"name", "target", "mnemonic", "build"}
	name := ""
	if grouped {
		name = "Grouped by "
	}
	name += fields[field]
	for key, value := range sel {
		name += fmt.Sprintf(", %s=%s", key, value)
	}
	return name
}

func (v operationList) renderTitle() string {
	return fmt.Sprintf("%s Operations (%s) %d", v.modeTitle(), fieldName(v.grouped, v.field, v.Select), len(v.opNames))
}

func (v operationList) Render() []ui.Drawable {
	v.list.Title = v.renderTitle()

	var rows []fmt.Stringer
	if v.grouped {
		rows = v.renderGrouped()
	} else {
		rows = v.renderItemized()
		now := time.Now()
		fence := func(e1, e2 fmt.Stringer) bool {
			stageEx1, stageEx2 := e1.(*stageEx), e2.(*stageEx)
			fence1, fence2 := now, now
			if stageEx1.done && stageEx2.done {
				return (stageEx1.final.Sub(stageEx1.fence) < stageEx2.final.Sub(stageEx2.fence)) != v.reversed
			}
			if !stageEx1.fence.IsZero() {
				fence1 = stageEx1.fence
			}
			if !stageEx2.fence.IsZero() {
				fence2 = stageEx2.fence
			}
			// should probably sort by just completion time
			if stageEx1.fence.IsZero() && stageEx2.fence.IsZero() {
				return stageEx1.name > stageEx2.name != v.reversed
			}
			return (fence1.Compare(fence2) < 0) != v.reversed
		}
		byExec(fence).Sort(rows)
	}
	v.list.Rows = rows
	v.list.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	v.list.WrapText = false
	v.list.SetRect(0, 0, 160, 30)

	content := []ui.Drawable{v.list}
	debug := client.NewParagraph()
	debug.SetRect(30, 30, 80, 40)
	debug.Text = fmt.Sprintf("token: %s, changed %v, stall: %v, stallStart: %v", v.fetchToken, v.fetchChanged, v.stall, v.stallStart)
	if v.debug {
		content = append(content, debug)
	}

	return content
}
