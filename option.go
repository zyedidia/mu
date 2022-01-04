package ned

import "fmt"

type Options struct {
	ed   *Editor
	opts map[string]*Option
}

type Option struct {
	val    interface{}
	valid  Validator
	update Updater
}

func (o *Options) Add(name string, valid Validator) {
	o.opts[name] = &Option{
		val:    nil,
		valid:  valid,
		update: nil,
	}
}

func (o *Options) AddOpt(name string, opt Option) {
	o.opts[name] = &opt
}

func (o *Options) Set(name string, v interface{}) error {
	if err := o.opts[name].valid(o.ed, v); err != nil {
		return err
	}
	o.opts[name].val = v
	if o.opts[name].update != nil {
		o.opts[name].update(o.ed, v)
	}
	return nil
}

func (o *Options) Get(opt string) interface{} {
	return o.opts[opt].val
}

type Updater func(e *Editor, v interface{})

type Validator func(e *Editor, v interface{}) error

func ValidBool(e *Editor, v interface{}) error {
	_, ok := v.(bool)
	if !ok {
		return fmt.Errorf("value is not a boolean")
	}
	return nil
}

func ValidInt(e *Editor, v interface{}) error {
	_, ok := v.(int)
	if !ok {
		return fmt.Errorf("value is not an int")
	}
	return nil
}

func ValidNonNegative(e *Editor, v interface{}) error {
	i, ok := v.(int)
	if !ok {
		return fmt.Errorf("value is not an int")
	}
	if i < 0 {
		return fmt.Errorf("value is less than zero")
	}
	return nil
}

func ValidString(e *Editor, v interface{}) error {
	_, ok := v.(string)
	if !ok {
		return fmt.Errorf("value is not a string")
	}
	return nil
}
