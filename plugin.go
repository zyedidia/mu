package mu

import (
	"log"

	"github.com/zyedidia/mu/pkg/tclutil"
)

func (e *Editor) initPlugins() {
	e.plugins.AddPackage("micro", map[string]any{
		"Editor": func() *Editor {
			return e
		},
		"PreHook":  tclutil.PreHook,
		"PostHook": tclutil.PostHook,
		"Command": func(cmd string, fn func(e *Editor, a []string)) {
			tclutil.Register(e.interp, cmd, fn, e, nil, nil)
		},
	})
	err := e.plugins.Load()
	if err != nil {
		log.Println("error loading plugins:", err)
	}
}
