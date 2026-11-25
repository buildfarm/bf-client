package main

import (
	"flag"
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
	a          *client.App
	v          view.View
	dimensions ui.Resize
	done       bool
	fps        int
	ups        float64
}

func (c *baseComponent) Open(args []string) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	tm.SetInputMode(tm.InputEsc)

	reapiHost := args[0]

	var ca string
	if len(args) > 1 {
		ca = args[1]
	}

	c.a = client.NewApp(reapiHost, ca)
	c.a.SetRates(c.fps, c.ups)
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
	lastFramesPerSecond := c.a.FramesPerSecond
	ticker := time.NewTicker(time.Second / time.Duration(c.a.FramesPerSecond)).C
	for !c.done {
		select {
		case e := <-uiEvents:
			c.handle(e)
			if lastFramesPerSecond != c.a.FramesPerSecond {
				ticker = time.NewTicker(time.Second / time.Duration(c.a.FramesPerSecond)).C
				lastFramesPerSecond = c.a.FramesPerSecond
			}
		case <-ticker:
			if c.a.UpdateCountdown == 0 {
				c.a.UpdateCountdown = c.a.FramesToSkip()
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
	fps := flag.Int("fps", client.DefaultFramesPerSecond, "maximum UI frames per second")
	upsFlag := flag.Float64("ups", 0, "maximum data updates per second (defaults to FPS and is capped at FPS)")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [options] reapi-host [ca-file]\n", flag.CommandLine.Name())
		fmt.Fprintln(out, "  reapi-host  Buildfarm gRPC address. Use grpcs:// for TLS.")
		fmt.Fprintln(out, "  ca-file     Optional CA certificate file for TLS.")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *fps < 1 || *fps > int(time.Second) {
		fmt.Fprintf(os.Stderr, "-fps must be between 1 and %d\n", time.Second.Nanoseconds())
		os.Exit(2)
	}
	ups := float64(*fps)
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "ups" {
			ups = *upsFlag
		}
	})
	if !client.ValidUpdatesPerSecond(ups) {
		fmt.Fprintln(os.Stderr, "-ups must be a finite positive number")
		os.Exit(2)
	}
	if flag.NArg() < 1 || flag.NArg() > 2 {
		flag.Usage()
		os.Exit(2)
	}

	args := flag.Args()
	var c client.Component = &baseComponent{fps: *fps, ups: ups}
	var nc client.Component = nil
	for c != nil {
		if nc != c {
			c.Open(args)
		}
		nc = c.Run()
		if nc != c {
			c.Close()
		}
		c = nc
	}
}
