package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"

	"github.com/werkt/bf-client/client"
	"github.com/werkt/bf-client/view"

	tm "github.com/nsf/termbox-go"
)

type component interface {
	open()
	close()
	handle(ui.Event)
	update() component
	render()
	done() bool
}

type baseComponent struct {
	a *client.App
	v view.View
}

func (c *baseComponent) open() {
	c.a.Connect()
}

func (c *baseComponent) close() {
	c.a.Conn.Close()
}

func (c *baseComponent) handle(e ui.Event) {
	c.v = c.v.Handle(e)
}

func (c *baseComponent) update() component {
	c.v.Update()
	return c
}

func (c baseComponent) render() {
	w := c.v.Render()

	f := widgets.NewParagraph()
	f.Text = fmt.Sprintf("Fetches: %d", c.a.Fetches)
	f.SetRect(0, 40, 20, 43)

	ui.Render(append(w, f)...)
}

func (c baseComponent) done() bool {
	return c.a.Done
}

func main() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	tm.SetInputMode(tm.InputEsc)
	defer ui.Close()

	redisHost, reapiHost := os.Args[1], os.Args[2]

	var ca string
	if len(os.Args) > 3 {
		ca = os.Args[3]
	}

	if !strings.Contains(redisHost, ":") {
		redisHost += ":6379"
	}

	a := client.NewApp(redisHost, reapiHost, ca)
	var c component = &baseComponent{
		a: a,
		v: view.NewQueue(a, 3),
	}

	c.open()

	uiEvents := ui.PollEvents()
	lastFrameLimit := a.FrameLimit
	ticker := time.NewTicker(time.Second / time.Duration(a.FrameLimit)).C
	for !c.done() {
		select {
		case e := <-uiEvents:
			c.handle(e)
			if lastFrameLimit != a.FrameLimit {
				ticker = time.NewTicker(time.Second / time.Duration(a.FrameLimit)).C
				lastFrameLimit = a.FrameLimit
			}
		case <-ticker:
			if a.UpdateCountdown == 0 {
				a.UpdateCountdown = a.SkipFrames
				a.Fetches = 0
				c = c.update()
			} else {
				a.UpdateCountdown--
			}
			c.render()
		}
	}

	c.close()
}
