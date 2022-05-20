package config

import (
	"strings"

	"github.com/pelletier/go-toml"
)

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
	var m map[string]interface{}
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
		m[k] = v
	}
	for _, ftopts := range o.ft {
		if strings.HasPrefix(ftopts.ft, "ft:") && ftopts.ft[3:] == ft {
			for k, v := range ftopts.opts {
				m[k] = v
			}
		}
	}
	return m
}
