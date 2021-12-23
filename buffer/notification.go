package buffer

var Notify chan struct{}

func init() {
	Notify = make(chan struct{})
}
