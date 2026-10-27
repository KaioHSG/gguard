package main

import (
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func checkICO(t *testing.T, ico []byte, wantSize int) {
	t.Helper()

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
	if w := int(ico[6]); w != wantSize {
		t.Errorf("entry width = %d, want %d", w, wantSize)
	}
	if h := int(ico[7]); h != wantSize {
		t.Errorf("entry height = %d, want %d", h, wantSize)
	}
	if bpp := binary.LittleEndian.Uint16(ico[12:]); bpp != 32 {
		t.Errorf("bpp = %d, want 32", bpp)
	}
	if off := binary.LittleEndian.Uint32(ico[18:]); off != 22 {
		t.Errorf("image offset = %d, want 22", off)
	}
	if size := binary.LittleEndian.Uint32(ico[14:]); int(size) != len(ico)-22 {
		t.Errorf("image size = %d, want %d", size, len(ico)-22)
	}
}

func TestEncodeICOHeader(t *testing.T) {
	checkICO(t, statusDotICO(StateOK), 16)

	img := iconImage()
	if img == nil {
		t.Fatal("cannot decode embedded tray icon")
	}
	checkICO(t, trayIconICO(StateOK), img.Bounds().Dx())
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

// TestRenderTrayPreview writes the composed tray icons as PNGs so they can be
// inspected visually. Skipped unless GGUARD_PREVIEW_DIR is set.
func TestRenderTrayPreview(t *testing.T) {
	dir := os.Getenv("GGUARD_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set GGUARD_PREVIEW_DIR to render previews")
	}

	for _, s := range []GuardState{StateOK, StateIdle, StateError, StateRunning} {
		img := iconImage()
		if img == nil {
			t.Fatal("cannot decode tray icon")
		}
		drawBadge(img, colorForState(s))

		f, err := os.Create(filepath.Join(dir, s.String()+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

var errTest = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
