package buffer_test

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/buffer/text"
	"github.com/zyedidia/mu/pkg/input"
)

var letters = []byte("\nabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randbytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return b
}

func check(want, got []byte, t *testing.T) {
	if !bytes.Equal(want, got) {
		t.Errorf("incorrect slices: want %s, got %s", string(want), string(got))
	}
}

func TestUndo(t *testing.T) {
	base := []byte("the quick brown fox")
	b, err := buffer.NewBuffer(input.NewReader(bytes.NewReader(base), ""), nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	const nedits = 200
	for i := 0; i < nedits; i++ {
		j := rand.Intn(2)
		switch j {
		case 0:
			b.Insert(rand.Intn(b.Len()), randbytes(20))
		case 1:
			x, y := rand.Intn(b.Len()), rand.Intn(b.Len())
			if y < x {
				x, y = y, x
			}
			b.Remove(x, y)
		}
		b.UndoBarrier()
	}

	snapshot := make([]byte, b.Len())
	copy(snapshot, b.Bytes())

	for i := 0; i < nedits; i++ {
		b.Undo()
	}

	check(base, b.Bytes(), t)

	for i := 0; i < nedits; i++ {
		b.Redo()
	}

	check(snapshot, b.Bytes(), t)
}

var (
	text1 = []byte("Lorem ipsum dolor sit amet, consectetuer adipiscing elit. Aenean commodo ligula eget dolor. Aenean massa. Cum sociis natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Donec quam felis, ultricies nec, pellentesque eu, pretium quis, sem. Nulla consequat massa quis enim. Donec pede justo, fringilla vel, aliquet nec, vulputate eget, arcu.")
	text2 = []byte("In enim justo, rhoncus ut, imperdiet a, venenatis vitae, justo. Nullam dictum felis eu pede mollis pretium. Integer tincidunt. Cras dapibus. ligula eget dolor. Aenean massa. Cum sociis natoque penatibus et magnis dis parturient montes, nascetur ridiculus mus. Donec quam felis, ultricies nec, Vivamus elementum semper nisi. Aenean vulputate eleifend tellus. Aenean leo ligula, porttitor eu, consequat vitae, eleifend ac, enim. Aliquam lorem ante, dapibus in, viverra quis, feugiat a, tellus. Phasellus viverra nulla ut metus varius laoreet. Quisque rutrum. Aenean imperdiet. Etiam ultricies nisi vel augue. Curabitur ullamcorper ultricies nisi.")
)

func TestDiff(t *testing.T) {
	base := text1
	b, err := buffer.NewBuffer(input.NewReader(bytes.NewReader(base), ""), nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	b.SetContent(text.NewBufferFromUTF8(text2, text.Options{}))

	check(text2, b.Bytes(), t)
	b.Undo()
	check(text1, b.Bytes(), t)
}
