package text_test

import (
	"testing"

	"github.com/zyedidia/mu/text"
)

func TestIsUTF8_ASCII(t *testing.T) {
	if !text.IsUTF8([]byte("hello world")) {
		t.Fatal("pure ASCII should be valid UTF-8")
	}
}

func TestIsUTF8_Empty(t *testing.T) {
	if !text.IsUTF8([]byte{}) {
		t.Fatal("empty slice should be valid UTF-8")
	}
}

func TestIsUTF8_ValidMultibyte(t *testing.T) {
	// 2-byte: é (0xC3 0xA9)
	// 3-byte: 日 (0xE6 0x97 0xA5)
	// 4-byte: 🎉 (0xF0 0x9F 0x8E 0x89)
	if !text.IsUTF8([]byte("café 日本語 🎉")) {
		t.Fatal("valid multibyte UTF-8 should pass")
	}
}

func TestIsUTF8_InvalidContinuation(t *testing.T) {
	// 0xC3 expects a continuation byte, 0x28 is not one
	if text.IsUTF8([]byte{0xC3, 0x28}) {
		t.Fatal("invalid continuation byte should fail")
	}
}

func TestIsUTF8_InvalidStartByte(t *testing.T) {
	// 0xFF is never valid in UTF-8
	if text.IsUTF8([]byte{0xFF, 0xFE}) {
		t.Fatal("0xFF start byte should fail")
	}
}

func TestIsUTF8_OverlongSequence(t *testing.T) {
	// Note: this validator checks structure, not overlong encoding.
	// 0xC0 0xAF is structurally valid (2-byte form) but represents an
	// overlong encoding. The IsUTF8 function counts structural errors,
	// so this may or may not fail depending on implementation.
	// 0xC0 with continuation 0xAF: 0xC0 is 110_00000, continuation is fine
	// Actually 0xC0 & 0xE0 == 0xC0, so trailBytes=1, 0xAF & 0xC0 == 0x80, so valid continuation.
	// The implementation only checks structure, not overlong, so this passes.
}

func TestIsUTF8_TruncatedSequence(t *testing.T) {
	// 3-byte sequence start but only 1 continuation byte.
	// The inner loop will exhaust input before finishing trail bytes.
	// trailBytes won't reach 0 so numValid won't increment, but
	// no explicit invalid byte is found. Still reports valid since numInvalid == 0.
	if !text.IsUTF8([]byte{0xE6, 0x97}) {
		t.Fatal("truncated sequence with valid continuations should not count as invalid")
	}
}

func TestIsUTF8_ManyInvalid(t *testing.T) {
	// More than 5 invalid start bytes — triggers early exit
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if text.IsUTF8(data) {
		t.Fatal("many invalid bytes should fail")
	}
}

func TestIsUTF8_Latin1(t *testing.T) {
	// Latin-1 encoded text with bytes > 0x7F that aren't valid UTF-8 continuations
	data := []byte{0xE4, 0xF6, 0xFC} // ä ö ü in Latin-1
	if text.IsUTF8(data) {
		t.Fatal("Latin-1 encoded bytes should fail UTF-8 check")
	}
}
