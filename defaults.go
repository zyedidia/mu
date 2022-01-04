package ned

import "fmt"

var defaults = map[string]*Option{
	"softwrap": &Option{
		val:   false,
		valid: ValidBool,
	},
	"wordwrap": &Option{
		val:   false,
		valid: ValidBool,
	},
	"tabsize": &Option{
		val:   4,
		valid: ValidNonNegative,
	},
	"colorscheme": &Option{
		val: "monokai",
		valid: func(e *Editor, v interface{}) error {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("value is not a string")
			}
			if s != "monokai" {
				return fmt.Errorf("invalid colorscheme")
			}
			return nil
		},
	},
	"mode": &Option{
		val: "normal",
		valid: func(e *Editor, v interface{}) error {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("value is not a string")
			}
			if e.modes == nil {
				return fmt.Errorf("no modes available")
			}
			if _, ok := e.modes[s]; !ok {
				return fmt.Errorf("mode %s does not exist", s)
			}
			return nil
		},
		update: func(e *Editor, v interface{}) {
			e.SetMode(v.(string))
		},
	},
}
