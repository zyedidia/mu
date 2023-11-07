package mu

import (
	lua "github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

func (e *Editor) initLua() {
	e.lua.L.SetGlobal("import", luar.New(e.lua.L, func(pkg string) *lua.LTable {
		return e.lua.Import(pkg)
	}))
}
