package main

import (
	"encoding/binary"
	"testing"
)

func TestEncodeICOHeader(t *testing.T) {
	ico := statusDotICO(StateOK)

	if len(ico) < 22 {
		t.Fatalf("ico too small: %d bytes", len(ico))
	}
	if got := binary.LittleEndian.Uint16(ico[0:]); got != 0 {
		t.Errorf("reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(ico[2:]); got != 1 {
		t.Errorf("type = %d, want 1 (icon)", got)
	}
	if got := binary.LittleEndian.Uint16(ico[4:]); got != 1 {
		t.Errorf("count = %d, want 1", got)
	}

	if w := ico[6]; w != 32 {
		t.Errorf("entry width = %d, want 32", w)
	}
	if h := ico[7]; h != 32 {
		t.Errorf("entry height = %d, want 32", h)
	}
	if bpp := binary.LittleEndian.Uint16(ico[12:]); bpp != 32 {
		t.Errorf("bpp = %d, want 32", bpp)
	}
	if off := binary.LittleEndian.Uint32(ico[18:]); off != 22 {
		t.Errorf("image offset = %d, want 22", off)
	}

	size := binary.LittleEndian.Uint32(ico[14:])
	if int(size) != len(ico)-22 {
		t.Errorf("image size = %d, want %d", size, len(ico)-22)
	}
}

func TestColorForState(t *testing.T) {
	cases := []struct {
		state   GuardState
		r, g, b uint8
	}{
		{StateOK, 0x22, 0xc5, 0x5e},
		{StateIdle, 0xea, 0xb3, 0x08},
		{StateError, 0xef, 0x44, 0x44},
		{StateRunning, 0x3b, 0x82, 0xf6},
	}
	for _, c := range cases {
		got := colorForState(c.state)
		if got.R != c.r || got.G != c.g || got.B != c.b || got.A != 0xff {
			t.Errorf("colorForState(%v) = %v, want rgb(%d,%d,%d)", c.state, got, c.r, c.g, c.b)
		}
	}
}

func TestAggregatePriority(t *testing.T) {
	tr := NewStatusTracker()
	tr.Add("a", "/a")
	tr.Add("b", "/b")

	if got := tr.Aggregate(); got != StateIdle {
		t.Fatalf("fresh tracker = %v, want idle", got)
	}

	tr.SetResult("a", nil)
	if got := tr.Aggregate(); got != StateOK {
		t.Fatalf("after ok = %v, want ok", got)
	}

	tr.SetRunning("b")
	if got := tr.Aggregate(); got != StateRunning {
		t.Fatalf("while running = %v, want running", got)
	}

	tr.SetResult("b", errTest)
	if got := tr.Aggregate(); got != StateError {
		t.Fatalf("after error = %v, want error", got)
	}
}

var errTest = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
