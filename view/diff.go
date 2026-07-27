package view

import (
	"maps"
	"iter"
	"errors"
	"io"
	"bufio"
	"sync"
	"context"
	"os"
	"fmt"
	"slices"
	"strings"
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/googleapis/longrunning"
	"google.golang.org/protobuf/encoding/protodelim"
	"github.com/buildfarm/bf-client/client"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
)

const ALIGNED_FIELDS int = 2

type diff struct {
	path string

	name string

	// uuid
	invocations [2]string

	ops [2][]*longrunning.Operation
	opNames map[string]*longrunning.Operation
	opPages [2]string
	opFiles [2]*os.File
	actionFiles [2]*os.File

	actions [2]map[string]reapi.Action
	commands map[string]reapi.Command
	digests [2]map[string][]string

	v View

	c client.Component

	mutex *sync.Mutex
	p *widgets.Paragraph
	l *client.List
	commandRows []fmt.Stringer
	alignedField int
	ui []ui.Drawable
	buffer string
	status string
	running bool
}

// split into create new and then load/create
func LoadDiff(c client.Component, path string, name string, v View) View {
	// load two invocations
	p := widgets.NewParagraph()
	l := client.NewList()
	l.Focused = false
	l.Title = "Commands"
	l.SetRect(2, 2, 120, 40)
	l.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	d := &diff {
		path: path,
		name: name,
		v: v,
		ops: [2][]*longrunning.Operation {
			[]*longrunning.Operation{},
			[]*longrunning.Operation{},
		},
		opPages: [2]string{ "", "" },
		c: c,
		mutex: &sync.Mutex{},
		running: true,
		p: p,
		l: l,
		ui: []ui.Drawable{ p },
	}

	b, err := os.ReadFile(d.entry("invocations"))
	if err != nil {
		panic(err)
	}
	i := strings.Split(string(b), "\n")
	// newline at eof makes this 3
	if len(i) != 3 {
		panic("invalid invocations list")
	}
	i0, i1 := i[0], i[1]

	f0, err := os.OpenFile(d.entry(i0 + ".ops"), os.O_RDWR, 0)
	if err != nil {
		panic(err)
	}
	f1, err := os.OpenFile(d.entry(i1 + ".ops"), os.O_RDWR, 0)
	if err != nil {
		panic(err)
	}

	p.SetRect(0, 0, 120, 20)
	p.WrapText = false
	d.invocations = [2]string{ i0, i1 }
	d.opFiles = [2]*os.File{ f0, f1 }
	d.digests = [2]map[string][]string {
		map[string][]string {},
		map[string][]string {},
	}

	// load existing ops entries
	go d.loadOperations(0)
	go d.loadOperations(1)

	return d
}

func CreateDiff(c client.Component, path string, i0 string, i1 string, v View) View {
	name := fmt.Sprintf("%s-%s", i0, i1)
	p := widgets.NewParagraph()
	l := client.NewList()
	l.Title = "Commands"
	l.SetRect(2, 2, 120, 40)
	l.SelectedRowStyle = ui.NewStyle(ui.ColorBlack, ui.ColorWhite)
	d := &diff {
		path: path,
		name: name,
		v: v,
		ops: [2][]*longrunning.Operation {
			[]*longrunning.Operation{},
			[]*longrunning.Operation{},
		},
		opPages: [2]string{ "", "" },
		c: c,
		mutex: &sync.Mutex{},
		running: true,
		p: p,
		l: l,
		ui: []ui.Drawable{ p },
	}

	dir := path + "/" + name
	err := os.Mkdir(dir, 0755)
	if err != nil {
		textEntry := NewTextEntry("Error", func(string) bool { return true; }, UISize(), v)
		textEntry.content = err.Error()
		textEntry.focus = false
		return textEntry
	}

	// phase initiate: fetch operations concurrently
	os.WriteFile(d.entry("invocations"), []byte(i0 + "\n" + i1 + "\n"), 0644)

	f0, err := os.Create(d.entry(i0 + ".ops"))
	if err != nil {
		textEntry := NewTextEntry("Error", func(string) bool { return true; }, UISize(), v)
		textEntry.content = err.Error()
		textEntry.focus = false
		return textEntry
	}
	f1, err := os.Create(d.entry(i1 + ".ops"))
	if err != nil {
		textEntry := NewTextEntry("Error", func(string) bool { return true; }, UISize(), v)
		textEntry.content = err.Error()
		textEntry.focus = false
		return textEntry
	}
	p.SetRect(0, 0, 120, 20)
	p.WrapText = false
	d.invocations = [2]string{ i0, i1 }
	d.opFiles = [2]*os.File{ f0, f1 }
	d.digests = [2]map[string][]string {
		map[string][]string {},
		map[string][]string {},
	}
	go d.fetchOperations(i0, 0)
	go d.fetchOperations(i1, 1)
	return d
}

func (d diff) entry(name string) string {
	return d.path + "/" + d.name + "/" + name
}

func digestString(d reapi.Digest) string {
	return fmt.Sprintf("%s/%d", d.Hash, d.SizeBytes)
}

func getActionDigest(op *longrunning.Operation) string {
	em, err := client.ExecuteOperationMetadata(op)
	if err != nil {
		panic(err)
	}
	return digestString(*em.ActionDigest)
}

func getActionDigests(ops []*longrunning.Operation) map[string]string {
	names := map[string]string{}
	for _, op := range ops {
		// we dealt with duplicates in java... why?
		names[getActionDigest(op)] = op.Name
	}
	return names
}

func difference(a map[string]string, b map[string]string) map[string]string {
	remaining := map[string]string{}
	for key, value := range a {
		_, ok := b[key]
		if !ok {
			remaining[key] = value
		}
	}
	return remaining
}

func (d *diff) loadActions(index int, f *os.File) map[string]reapi.Action {
	actions := map[string]reapi.Action{}
	b := bufio.NewReader(f)
	for ;; {
		var breq reapi.BatchReadBlobsRequest
		err := protodelim.UnmarshalFrom(b, &breq)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		if err != nil {
			// eof
			return actions
		}
		var br reapi.BatchReadBlobsResponse
		err = protodelim.UnmarshalFrom(b, &br)
		if err != nil {
			panic(err)
		}
		d.processActions(&breq, &br, actions)
	}
}

func (d *diff) loadCommands(f *os.File) map[string]reapi.Command {
	commands := map[string]reapi.Command{}
	b := bufio.NewReader(f)
	for ;; {
		var breq reapi.BatchReadBlobsRequest
		err := protodelim.UnmarshalFrom(b, &breq)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		if err != nil {
			// eof
			return commands
		}
		var br reapi.BatchReadBlobsResponse
		err = protodelim.UnmarshalFrom(b, &br)
		if err != nil {
			panic(err)
		}
		d.processCommands(&breq, &br, commands)
	}
}

func (d *diff) processActions(req *reapi.BatchReadBlobsRequest, res *reapi.BatchReadBlobsResponse, actions map[string]reapi.Action) {
	// unmarshal the actions from the responses
	// record the digests for each
	for i, d := range req.Digests {
		key := fmt.Sprintf("%s/%d", d.Hash, d.SizeBytes)
		var a reapi.Action
		err := proto.Unmarshal(res.Responses[i].Data, &a)
		if err != nil {
			panic(err)
		}
		actions[key] = a
	}
}

func (d *diff) processCommands(req *reapi.BatchReadBlobsRequest, res *reapi.BatchReadBlobsResponse, commands map[string]reapi.Command) {
	// unmarshal the commands from the responses
	// record the digests for each
	for i, d := range req.Digests {
		key := fmt.Sprintf("%s/%d", d.Hash, d.SizeBytes)
		var c reapi.Command
		err := proto.Unmarshal(res.Responses[i].Data, &c)
		if err != nil {
			panic(err)
		}
		commands[key] = c
	}
}

func (d *diff) processDirectories(req *reapi.BatchReadBlobsRequest, res *reapi.BatchReadBlobsResponse, directories map[string]reapi.Directory) {
	// unmarshal the directories from the responses
	// record the digests for each
	for i, d := range req.Digests {
		key := fmt.Sprintf("%s/%d", d.Hash, d.SizeBytes)
		var c reapi.Directory
		err := proto.Unmarshal(res.Responses[i].Data, &c)
		if err != nil {
			panic(err)
		}
		directories[key] = c
	}
}

func toDigests(strings iter.Seq[string]) []reapi.Digest {
	l := 0
	for _ = range strings {
		l++
	}
	digests := make([]reapi.Digest, l)
	i := 0
	for s := range strings {
		digests[i] = client.ParseREDigest(s)
		i++
	}
	return digests
}

func toDigestPointers(digests []reapi.Digest) []*reapi.Digest {
	pointers := make([]*reapi.Digest, len(digests))
	for i := range digests {
		pointers[i] = &digests[i]
	}
	return pointers
}

func (d *diff) loadOrListCommands(df reapi.DigestFunction_Value) {
	digestStrings := map[string]bool{}
	for i := 0; i < 2; i++ {
		for _, action := range d.actions[i] {
			digestStrings[digestString(*action.CommandDigest)] = true
		}
	}

	var commands map[string]reapi.Command
	i0, i1 := d.invocations[0], d.invocations[1]
	name := fmt.Sprintf("%s-%s.commands", i0, i1)
	path := d.entry(name)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		d.mutex.Lock()
		d.status = "\nLoading Commands"
		d.mutex.Unlock()
		commands = d.loadCommands(f)
	} else {
		d.mutex.Lock()
		d.status = "\nListing Commands"
		d.mutex.Unlock()
		// chunk the digests
		f, err = os.Create(path)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		c := reapi.NewContentAddressableStorageClient(d.c.App().Conn)
		// absolutely no reason we need to wait for each of these chunks in turn

		// gymnastics suck here
		digests := toDigests(maps.Keys(digestStrings))

		commands = map[string]reapi.Command{}
		// no good static chunk size
		limit := int64(2 * 1024 * 1024)
		chunk := []*reapi.Digest{}
		for i, digest := range digests {
			last := i == len(digests) - 1
			if limit > digest.SizeBytes && !last {
				limit -= digest.SizeBytes
				chunk = append(chunk, &digests[i])
				continue
			}
			req := &reapi.BatchReadBlobsRequest{
				InstanceName: d.c.App().Instance,
				Digests:      chunk,
				DigestFunction: df,
			}
			r, err := c.BatchReadBlobs(context.Background(), req)
			if err != nil {
				panic(err)
			}
			_, err = protodelim.MarshalTo(f, req)
			if err != nil {
				panic(err)
			}
			_, err = protodelim.MarshalTo(f, r)
			if err != nil {
				panic(err)
			}
			d.processCommands(req, r, commands)
			limit = int64(2 * 1024 * 1024) - digest.SizeBytes
			chunk = []*reapi.Digest{&digests[i]}
		}
	}
	d.mutex.Lock()
	d.commands = commands
	d.status = "\nCommands Loaded"
	d.mutex.Unlock()
}

type inputDir struct {
	parent *inputDir
	name string
	d [2]reapi.Digest
	c *command
}

func (id inputDir) path(name string) string {
	if id.parent == nil {
		return name
	}
	return id.parent.path(id.name + "/" + name)
}

type fileDiff struct {
	path string
	d [2]reapi.Digest
}

func (d *diff) diffInputs(commands []fmt.Stringer, df reapi.DigestFunction_Value) {
	// for all commands that currently exist in the list
	dirs := []inputDir{}
	for _, s := range commands {
		c := s.(*command)
		a0, a1 := d.actions[0][getActionDigest(c.op[0])], d.actions[1][getActionDigest(c.op[1])]
		// command must already match, pump directory into channel
		d := inputDir{ d: [2]reapi.Digest{ *a0.InputRootDigest, *a1.InputRootDigest }, c: c }
		dirs = append(dirs, d)
	}

	directories := map[string]reapi.Directory{}
	c := reapi.NewContentAddressableStorageClient(d.c.App().Conn)
	limit := int64(2 * 1024 * 1024)
	chunk := []*reapi.Digest{}
	chunkDirs := []inputDir{}
	total := len(dirs)
	completed := 0
	for len(dirs) > 0 {
		id, dirs := dirs[0], dirs[1:]
		last := len(dirs) == 0
		size := id.d[0].SizeBytes + id.d[1].SizeBytes

		if limit > size || last {
			limit -= size
			// could be over limit, but we're padded...

			// deduppppp...
			chunk = append(chunk, &id.d[0])
			chunk = append(chunk, &id.d[1])
			chunkDirs = append(chunkDirs, id)
			d.mutex.Lock()
			d.status = fmt.Sprintf("\nAggregating, %d remaining, %d", limit, len(chunk))
			d.mutex.Unlock()
			if !last {
				continue
			}
		}

		req := &reapi.BatchReadBlobsRequest{
			InstanceName: d.c.App().Instance,
			Digests: chunk,
			DigestFunction: df,
		}
		d.mutex.Lock()
		d.status += fmt.Sprintf("\nBatchReadBlobs(%d) %d/%d", len(chunk), completed, total)
		d.mutex.Unlock()
		r, err := c.BatchReadBlobs(context.Background(), req)
		if err != nil {
			panic(err)
		}
		completed += len(chunk)
		d.mutex.Lock()
		d.status += "\nProcessingDirectories"
		d.mutex.Unlock()
		d.processDirectories(req, r, directories)
		limit = int64(2 * 1024 * 1024)
		chunk = []*reapi.Digest{}

		// needs a complicated dedup setup

		for i, id := range chunkDirs {
			// locate the difference
			dir0 := directories[digestString(*req.Digests[i * 2 + 0])]
			dir1 := directories[digestString(*req.Digests[i * 2 + 1])]
			for j, fn0 := range dir0.Files {
				fn1 := dir1.Files[j]
				if fn0.Name != fn1.Name {
					panic("file names don't match")
				}
				if fn0.Digest != fn1.Digest {
					// need the path to the file, the digests for each
					id.c.files = append(id.c.files, fileDiff{ path: id.path(fn0.Name), d: [2]reapi.Digest{ *fn0.Digest, *fn1.Digest } })
				}
			}

			for j, dn0 := range dir0.Directories {
				dn1 := dir0.Directories[j]
				if dn0.Name != dn1.Name {
					panic("directory names don't match")
				}
				if dn0.Digest != dn1.Digest {
					cd := inputDir{ parent: &chunkDirs[i], d: [2]reapi.Digest{ *dn0.Digest, *dn1.Digest }, c: id.c }
					dirs = append(dirs, cd)
					total++
				}
			}
		}
		chunkDirs = []inputDir{}
	}
}

func (d *diff) loadOrListActions(index int, digestStrings iter.Seq[string], df reapi.DigestFunction_Value) {
	other := 1 - index
	i0, i1 := d.invocations[index], d.invocations[other]
	name := fmt.Sprintf("%s-%s.actions", i0, i1)
	var actions map[string]reapi.Action
	path := d.entry(name)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		actions = d.loadActions(index, f)
	} else {
		// chunk the digests
		f, err = os.Create(path)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		actions = map[string]reapi.Action{}
		c := reapi.NewContentAddressableStorageClient(d.c.App().Conn)
		// absolutely no reason we need to wait for each of these chunks in turn

		// gymnastics suck here
		digests := toDigests(digestStrings)

		chunkSize := 10000 // safe-ish for action size 180 under 2MiB response
		for chunk := range slices.Chunk(toDigestPointers(digests), chunkSize) {
			req := &reapi.BatchReadBlobsRequest{
				InstanceName: d.c.App().Instance,
				Digests:      chunk,
				DigestFunction: df,
			}
			r, err := c.BatchReadBlobs(context.Background(), req)
			if err != nil {
				panic(err)
			}
			_, err = protodelim.MarshalTo(f, req)
			if err != nil {
				panic(err)
			}
			_, err = protodelim.MarshalTo(f, r)
			if err != nil {
				panic(err)
			}
			d.processActions(req, r, actions)
		}
	}
	d.mutex.Lock()
	d.actions[index] = actions
	done := len(d.actions[other]) != 0
	d.mutex.Unlock()

	if done {
		d.mutex.Lock()
		d.buffer = d.status
		d.mutex.Unlock()

		d.loadOrListCommands(df)

		d.mutex.Lock()
		d.status = "\nIndexing operations"
		d.mutex.Unlock()

		commandKeyOps := [2]map[string]string{
			map[string]string{},
			map[string]string{},
		}
		for i := range d.ops {
			for _, op := range d.ops[i] {
				actionDigestString := getActionDigest(op)
				action, ok := d.actions[i][actionDigestString]
				if ok {
					commandKey := fmt.Sprintf("%s-%s", digestString(*action.CommandDigest), client.RequestMetadata(op).ConfigurationId)
					d.mutex.Lock()
					d.status = "\nIndexing operations\n" + commandKey
					d.mutex.Unlock()
					commandKeyOps[i][commandKey] = op.Name
				}
			}
		}
		unmatchedAB := difference(commandKeyOps[0], commandKeyOps[1])
		unmatchedBA := difference(commandKeyOps[1], commandKeyOps[0])
		matched := len(commandKeyOps[0]) - len(unmatchedAB)
		for key, _ := range unmatchedAB {
			delete(commandKeyOps[0], key)
		}
		for key, _ := range unmatchedBA {
			delete(commandKeyOps[1], key)
		}
		d.mutex.Lock()
		d.buffer += fmt.Sprintf("\nOperation Index Complete: %d matched, mismatched: %d A->B, %d B->A", matched, len(unmatchedAB), len(unmatchedBA))
		d.status = "Indexing mismatched operations' outputs"
		d.mutex.Unlock()

		d.indexNamesOutputs(maps.Values(commandKeyOps[0]), d.digests[0])
		d.indexNamesOutputs(maps.Values(commandKeyOps[1]), d.digests[1])

		d.commandRows = d.makeCommands(commandKeyOps)

		// d.diffInputs(commands, df)
		// locate all inputs that differ between them
		//   fetched via a maintained cache of directory inputs
		//   and batched with duplicates removed for fetches and buses leaving at the fill mark

		d.l.Rows = d.commandRows
		d.l.Title += fmt.Sprintf(" %d", len(d.l.Rows))

		d.mutex.Lock()
		d.l.Focused = true
		d.ui = []ui.Drawable { d.p, d.l }
		d.mutex.Unlock()

		// we have a list of actions that didn't match
		// we need to decide upon a hierarchy that produced them
		// for now, hitting enter on a command should present a display that indicates the differences between them

		// ok we have some indices now that should account for the funky ops that don't match
		// we could determine remainders
		// or we could create a more interesting UI that lets us select fields to align, and then subselect/inspect
	}
}

func (d *diff) alignOps() {
	opNames := map[string]*longrunning.Operation{}
	for _, ops := range d.ops {
		for _, op := range ops {
			opNames[op.Name] = op
		}
	}

	d.mutex.Lock()
	d.opNames = opNames
	d.status = "\nLoading Action Digests"
	d.mutex.Unlock()

	a0 := getActionDigests(d.ops[0])
	a1 := getActionDigests(d.ops[1])

	d.mutex.Lock()
	d.status = "\nDiffing Action Digests"
	d.mutex.Unlock()

	f0 := difference(a0, a1)
	f1 := difference(a1, a0)

	d.mutex.Lock()
	d.status = fmt.Sprintf("\n%[3]d %[1]s -> %[2]s\n%[4]d %[2]s -> %[1]s", d.invocations[0], d.invocations[1], len(f0), len(f1))
	d.mutex.Unlock()

	go d.loadOrListActions(0, maps.Keys(f0), reapi.DigestFunction_BLAKE3)
	go d.loadOrListActions(1, maps.Keys(f1), reapi.DigestFunction_BLAKE3)
}

func (d *diff) indexExecutionOutputs(o *longrunning.Operation, digests map[string][]string) {
	switch r := o.Result.(type) {
	case *longrunning.Operation_Response:
		er := &reapi.ExecuteResponse{}
		if r.Response.MessageIs(er) {
			err := ptypes.UnmarshalAny(r.Response, er)
			if err != nil {
				panic(err)
			}
			result := er.Result
			for _, of := range result.OutputFiles {
				ds := digestString(*of.Digest)
				ol, ok := digests[ds]
				if !ok {
					ol = []string{}
				}
				digests[ds] = append(ol, o.Name)
			}
		}
	}
}

func (d *diff) indexNamesOutputs(opNames iter.Seq[string], digests map[string][]string) {
	for name := range opNames {
		d.indexExecutionOutputs(d.opNames[name], digests)
	}
}

func (d *diff) indexOutputs(ops []*longrunning.Operation, digests map[string][]string) {
	for _, o := range ops {
		d.indexExecutionOutputs(o, digests)
	}
}

func (d *diff) loadOperations(index int) {
	invocation := d.invocations[index]
	done := false
	f := d.opFiles[index]
	stat, err := f.Stat()
	if err != nil {
		panic(err)
	}
	size := stat.Size()
	if size == 0 {
		d.fetchOperations(invocation, index)
		return
	}
	b := bufio.NewReader(f)
	ops := d.ops[index]
	for d.running {
		r := longrunning.ListOperationsResponse{}
		err := protodelim.UnmarshalFrom(b, &r)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		if err == nil {
			// faulty now
			ops = slices.Concat(ops, r.Operations)
			d.mutex.Lock()
			d.ops[index] = ops
			d.opPages[index] = r.NextPageToken
			d.mutex.Unlock()

			// this is unbelievably costly
			// d.indexOutputs(ops, d.digests[index])
		} else {
			// not done fetching?
			d.fetchOperations(invocation, index)
			return
		}

		// at eof?
		if r.NextPageToken == "" {
			// switch modes to action load/fetch
			f.Close()
			d.mutex.Lock()
			d.opFiles[index] = nil
			done = d.opFiles[1 - index] == nil
			d.mutex.Unlock()
			break
		}
	}
	if done {
		// we're the winner
		d.alignOps()
	}
}

func (d *diff) fetchOperations(invocation string, index int) {
	f := d.opFiles[index]
	ops := d.ops[index]
	done := false
	for d.running {
		req := &longrunning.ListOperationsRequest{
			Name:      d.c.App().Instance + "/executions",
			Filter:    "toolInvocationId=" + invocation,
			PageSize:  100,
			PageToken: d.opPages[index],
		}
		c := longrunning.NewOperationsClient(d.c.App().Conn)
		r, err := c.ListOperations(context.Background(), req)
		if err != nil {
			panic(err)
		}
		ops = slices.Concat(ops, r.Operations)
		// serialize the r
		// push to file
		_, err = protodelim.MarshalTo(f, r)
		d.mutex.Lock()
		d.ops[index] = ops
		d.opPages[index] = r.NextPageToken
		d.mutex.Unlock()
		// d.indexOutputs(ops, d.digests[index])
		if r.NextPageToken == "" {
			f.Close()
			d.mutex.Lock()
			d.opFiles[index] = nil
			done = d.opFiles[1 - index] == nil
			d.mutex.Unlock()
			break
		}
	}
	if done {
		d.alignOps()
	}
}

func (d diff) Render() []ui.Drawable {
	return d.ui
}

type command struct {
	d *diff
	op [2]*longrunning.Operation
	files []fileDiff
}

func (c command) view() View {
	return NewOperationDiff(c.d.c, c.d, c.op, reapi.DigestFunction_BLAKE3)
}

func (c command) String() string {
	ops := fmt.Sprintf("%s - %s", c.op[0].Name, c.op[1].Name)
	switch c.d.alignedField {
	case 0: // ops
		return ops
	case 1: // mnemonic target
		m0, m1 := client.RequestMetadata(c.op[0]), client.RequestMetadata(c.op[1])
		if m0.ActionMnemonic != m1.ActionMnemonic || m0.TargetId != m1.TargetId {
			panic(ops)
		}
		return fmt.Sprintf("%s %s", m0.ActionMnemonic, m0.TargetId)
	default:
		return "unknown field"
	}
}

func (d *diff) makeCommands(commandKeyOps [2]map[string]string) []fmt.Stringer {
	commands := []fmt.Stringer{}
	for key, op0Name := range commandKeyOps[0] {
		op1Name := commandKeyOps[1][key]
		commands = append(commands, &command { d: d, op: [2]*longrunning.Operation{ d.opNames[op0Name], d.opNames[op1Name] }})
	}
	return commands
}

func (d *diff) Update() {
	d.mutex.Lock()
	n0, p0, n1, p1 := len(d.ops[0]), d.opPages[0], len(d.ops[1]), d.opPages[1]
	status := d.status
	buffer := d.buffer
	d.mutex.Unlock()
	d.p.Text = fmt.Sprintf("%d %s\n%d %s", n0, p0, n1, p1) + buffer + status
}

func (d *diff) setFieldFilter(filter string) bool {
	if filter == "" {
		d.l.Rows = d.commandRows
	} else {
		rows := []fmt.Stringer{}
		for _, row := range d.commandRows {
			if strings.Contains(row.String(), filter) {
				rows = append(rows, row)
			}
		}
		d.l.Rows = rows
	}
	return true
}

func saveList(l *client.List, name string, c client.Component, v View) View {
	path := os.Getenv("HOME") + "/" + name
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	for _, s := range l.Rows {
		fmt.Fprintln(file, s.String())
	}
	return NewMessage("List Saved to " + path, c, v)
}

func (d *diff) Handle(e ui.Event) View {
	switch e.ID {
	case "<Escape>", "q", "<C-c>":
		d.running = false
		return d.v
	case "<Down>", "j":
		if d.l.Focused {
			d.l.ScrollDown()
		}
	case "<Up>", "k":
		if d.l.Focused {
			d.l.ScrollUp()
		}
	case "S":
		if d.l.Focused {
			return saveList(d.l, d.name + "-commands.txt", d.c, d)
		}
	case "<Left>", "h":
		d.alignedField = (d.alignedField + ALIGNED_FIELDS - 1) % ALIGNED_FIELDS
	case "<Right>", "l":
		d.alignedField = (d.alignedField + 1) % ALIGNED_FIELDS
	case "<Enter>":
		if d.l.Focused {
			return d.l.Rows[d.l.SelectedRow].(*command).view()
		}
	case "/":
		return NewTextEntry("filter field", d.setFieldFilter, UISize(), d)
	}
	return d
}
