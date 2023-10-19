package config

import (
	"log"
	"strings"

	"github.com/pelletier/go-toml"
	"github.com/zyedidia/glob"
)

var globals = map[string]bool{
	"theme":     true,
	"clipboard": true,
}

type ftopts struct {
	ft   string
	opts map[string]interface{}
}

type Options struct {
	top map[string]interface{}
	ft  []ftopts
}

func LoadOptions(data []byte) (*Options, error) {
	var optmap map[string]interface{}
	err := toml.Unmarshal(data, &optmap)
	if err != nil {
		return nil, err
	}
	opts := &Options{
		top: make(map[string]interface{}),
	}
	for k, v := range optmap {
		switch v := v.(type) {
		case map[string]interface{}:
			opts.ft = append(opts.ft, ftopts{
				ft:   k,
				opts: v,
			})
		default:
			opts.top[k] = v
		}
	}
	return opts, err
}

func (o *Options) ToToml() ([]byte, error) {
	m := make(map[string]interface{})
	for k, v := range o.top {
		m[k] = v
	}
	for _, ftopts := range o.ft {
		m[ftopts.ft] = ftopts.opts
	}
	return toml.Marshal(m)
}

func (o *Options) LocalOptions(path, ft string) map[string]interface{} {
	m := make(map[string]interface{})
	for k, v := range o.top {
		if globals[k] {
			continue
		}
		m[k] = v
	}
	for _, ftopts := range o.ft {
		if strings.HasPrefix(ftopts.ft, "glob:") {
			globstr := ftopts.ft[5:]
			if rgx, err := glob.Compile(globstr); err != nil {
				log.Printf("error compiling glob %s: %v\n", globstr, err)
				continue
			} else if !rgx.MatchString(path) {
				continue
			}
			// glob matches, fall through to copy the map into m
		} else {
			if ft != ftopts.ft {
				continue
			}
			// fall through
		}

		for k, v := range ftopts.opts {
			if globals[k] {
				continue
			}
			m[k] = v
		}
	}
	return m
}
