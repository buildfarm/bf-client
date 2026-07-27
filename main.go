package main

import (
	"fmt"
	"log"
	"os"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"

	"github.com/buildfarm/bf-client/client"
	"github.com/buildfarm/bf-client/view"

	tm "github.com/nsf/termbox-go"
)

type baseComponent struct {
	a *client.App
	v view.View
	dimensions ui.Resize
	done bool
}

func (c *baseComponent) Open(args []string) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	tm.SetInputMode(tm.InputEsc)

	reapiHost := args[1]

	var ca string
	if len(args) > 2 {
		ca = args[2]
	}

	c.a = client.NewApp(reapiHost, ca)
	c.v = view.NewQueue(c, 3)

	c.a.Connect()
}

func (c *baseComponent) Close() {
	ui.Close()

	c.a.Conn.Close()
}

func (c *baseComponent) Run() client.Component {
	c.dimensions = view.UISize()
	uiEvents := ui.PollEvents()
	lastFrameLimit := c.a.FrameLimit
	ticker := time.NewTicker(time.Second / time.Duration(c.a.FrameLimit)).C
	for !c.done {
		select {
		case e := <-uiEvents:
			c.handle(e)
			if lastFrameLimit != c.a.FrameLimit {
				ticker = time.NewTicker(time.Second / time.Duration(c.a.FrameLimit)).C
				lastFrameLimit = c.a.FrameLimit
			}
		case <-ticker:
			if c.a.UpdateCountdown == 0 {
				c.a.UpdateCountdown = c.a.SkipFrames
				c.a.Fetches = 0
				c.update()
			} else {
				c.a.UpdateCountdown--
			}
			c.render()
		}
	}
	return nil
}

func (c *baseComponent) App() *client.App {
	return c.a
}

func (c *baseComponent) Finish() {
	c.done = true
}

func (c *baseComponent) Dimensions() ui.Resize {
	return c.dimensions
}

func (c *baseComponent) handle(e ui.Event) {
	if e.ID == "<Resize>" {
		c.dimensions = e.Payload.(ui.Resize)
	}
	c.v = c.v.Handle(e)
}

func (c *baseComponent) update() {
	c.v.Update()
}

func (c baseComponent) render() {
	w := c.v.Render()

	f := widgets.NewParagraph()
	f.Text = fmt.Sprintf("Fetches: %d", c.a.Fetches)
	f.SetRect(0, 40, 20, 43)

	ui.Render(append(w, f)...)
}

func main() {
	var c client.Component
	c = &baseComponent {}
	var nc client.Component = nil
	for c != nil {
		if nc != c {
			c.Open(os.Args)
		}
		nc = c.Run()
		if nc != c {
			c.Close()
		}
		c = nc
	}
}
