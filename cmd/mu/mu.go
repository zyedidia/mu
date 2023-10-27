package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/go-errors/errors"
	"github.com/micro-editor/tcell/v2"
	"github.com/zyedidia/mu"
	"github.com/zyedidia/mu/build"
	"github.com/zyedidia/mu/pkg/theme"
)

type Map[K comparable, V any] map[K]V

func (m Map[K, V]) Get(k K) (V, bool) {
	v, ok := m[k]
	return v, ok
}

func (m Map[K, V]) Put(k K, v V) {
	m[k] = v
}

func EnterToContinue() {
	fmt.Print("Press enter to continue")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

const errmsg = `Please report this issue at https://github.com/zyedidia/micro/issues.`

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var showstats = flag.Bool("stats", false, "create performance statistics files")
var version = flag.Bool("version", false, "show version")

var stats Stats

func exit(code int) {
	if *cpuprofile != "" {
		pprof.StopCPUProfile()
	}
	if *showstats {
		fmt.Print(stats.String())
	}
	os.Exit(code)
}

func main() {
	f, err := os.Create(filepath.Join("/tmp", "mu.log"))
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	} else {
		defer f.Close()
		log.SetOutput(f)
	}

	flag.Parse()

	if *version {
		fmt.Println(build.Version)
		return
	}

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	args := flag.Args()

	s, e := tcell.NewScreen()
	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		exit(1)
	}
	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		exit(1)
	}

	defer func() {
		if err := recover(); err != nil {
			s.Fini()
			fmt.Fprintf(os.Stderr, "%s\n%v\n%s\n", "a fatal error occurred", errors.Wrap(err, 2).ErrorStack(), errmsg)
			exit(1)
		}
	}()

	w, h := s.Size()

	var ed *mu.Editor
	if len(args) > 0 {
		ed, err = mu.NewEditorFromPath(args[0], w, h, s)
	} else {
		ed, err = mu.NewEditor(w, h, s)
	}
	if err != nil {
		s.Fini()
		fmt.Fprintln(os.Stderr, err)
		exit(1)
	}

	draw := func(vx, vy int, mainc rune, combc []rune, style theme.Style) {
		s.SetContent(vx, vy, mainc, combc, tcellStyle(style))
	}
	fill := func(x rune, style theme.Style) {
		s.Fill(x, tcellStyle(style))
	}

	cursor := func(x, y int) {
		s.ShowCursor(x, y)
	}

	evs := make(chan tcell.Event)

	ed.Display(fill, draw, cursor)
	s.Show()

	go func() {
		for {
			evs <- s.PollEvent()
		}
	}()

	// Set up a signal receiver so we can exit gracefully if the user/OS shuts
	// us down (closing the screen, saving backups, etc.).
	sigterm := make(chan os.Signal, 1)
	sigint := make(chan os.Signal, 1)
	quit := make(chan struct{}, 1)
	terminate := make(chan int, 1)
	signal.Notify(sigterm, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGABRT)
	signal.Notify(sigint, syscall.SIGINT)

	go func() {
		for {
			select {
			case <-sigterm:
				log.Println("received kill signal")
				quit <- struct{}{}

				// if editor does not gracefully shut down in 1 second then we kill
				// ourselves from this goroutine
				time.Sleep(1 * time.Second)
				log.Println("force killing self")
				exit(1)
			case <-sigint:
				// do nothing
			}
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				s.Fini()
				fmt.Fprintf(os.Stderr, "%s\n%v\n%s\n", "a fatal error occurred", errors.Wrap(err, 2).ErrorStack(), errmsg)
				exit(1)
			}
		}()

	loop:
		for {
			select {
			case err := <-ed.Errors:
				if errors.Is(err, mu.ErrQuit) {
					break loop
				}
				var pe mu.PanicErr
				if errors.As(err, &pe) {
					s.Suspend()
					fmt.Fprintln(os.Stderr, err)
					EnterToContinue()
					s.Resume()
					ed.Display(fill, draw, cursor)
					s.Show()
				}
			case f := <-ed.Suspend:
				s.Suspend()
				f()
				<-ed.Resume
				s.Resume()
				ed.Display(fill, draw, cursor)
				s.Show()
			case <-ed.Redraw:
				start := time.Now()
				ed.Display(fill, draw, cursor)
				s.Show()
				stats.AddRedrawTime(time.Since(start))
			case <-quit:
				break loop
			}
		}
		s.Fini()
		terminate <- 0
	}()

	for {
		select {
		case ev := <-evs:
			start := time.Now()
			ed.HandleEvent(ev)
			stats.AddEventTime(time.Since(start))
		case code := <-terminate:
			exit(code)
		}

		stats.SampleAlloc()
	}
}

func tcellColor(color theme.Color) tcell.Color {
	if !color.Valid() {
		return tcell.ColorDefault
	}
	if color.IsRGB() {
		return tcell.NewHexColor(color.Hex())
	}
	return tcell.PaletteColor(color.Palette())
}

func tcellStyle(style theme.Style) tcell.Style {
	return tcell.StyleDefault.
		Foreground(tcellColor(style.Fg)).
		Background(tcellColor(style.Bg)).
		Attributes(tcell.AttrMask(style.Attr)) // small cheat
}
