package buffer_test

import (
	"strings"
	"testing"

	"github.com/zyedidia/mu/buffer"
	"github.com/zyedidia/mu/pkg/input"
)

func TestFiletypeDetect(t *testing.T) {
	tests := []struct {
		content string
		fname   string
		want    string
	}{
		{"#!/bin/bash\n", "test.sh", "shell"},
		{"#!/bin/bash\n", "test", "shell"},
		{"", "test.sh", "shell"},
		{"", "test.go", "go"},
	}

	for _, tt := range tests {
		t.Run(tt.fname, func(t *testing.T) {
			in := input.NewReader(strings.NewReader(tt.content), tt.fname)
			b, err := buffer.NewBuffer(in, nil, buffer.Options{})
			if err != nil {
				t.Fatal(err)
			}

			ft := b.Filetype()
			if ft != tt.want {
				t.Fatalf("incorrect filetype: got %s, want %s", ft, tt.want)
			}
		})
	}
}
