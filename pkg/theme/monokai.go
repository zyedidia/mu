package theme

import "log"

var monokaiYAML = `
default:
    fg: "#F8F8F2"
    bg: "#282828"
comment:
    fg: "#75715E"
    bg: "#282828"
constant:
    fg: "#AE81FF"
    bg: "#282828"
constant.string:
    fg: "#E6DB74"
    bg: "#282828"
constant.string.char:
    fg: "#BDE6AD"
    bg: "#282828"
constant.string.escape:
    fg: "#AE81FF"
    bg: "#282828"
symbol.tag:
    fg: "#AE81FF"
    bg: "#282828"
preproc:
    fg: "#CB4B16"
    bg: "#282828"
function:
    fg: "#A6E22E"
    bg: "#282828"
keyword:
    fg: "#F92672"
    bg: "#282828"
type:
    fg: "#66D9EF"
    bg: "#282828"
special:
    fg: "#A6E22E"
    bg: "#282828"
underlined:
    fg: "#D33672"
    bg: "#282828"
    attr: ["underline"]
error:
    fg: "#CB4B16"
    bg: "#282828"
    attr: ["bold"]
todo:
    fg: "#D33682"
    bg: "#282828"
    attr: ["bold"]
line-number:
    fg: "#AAAAAA"
    bg: "#323232"
statusline:
    fg: "#F8F8F2"
    bg: "#282828"
hidden-char:
    fg: "#505050"
    bg: "#282828"
`

var Monokai *Theme

func init() {
	Monokai = new(Theme)
	err := Monokai.LoadYAML([]byte(monokaiYAML))
	if err != nil {
		log.Println(err)
	}
}
