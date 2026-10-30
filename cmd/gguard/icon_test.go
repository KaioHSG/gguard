package main

import (
	"encoding/binary"
	"image"
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
		writePNG(t, filepath.Join(dir, s.String()+".png"), img)

		// The menu dot, blown up so the margin around it is visible: margin 1
		// puts the anti-aliased edge on the last pixel, which the menu renders
		// edge to edge.
		for _, margin := range []int{1, 2, 3} {
			dot := dotImage(colorForState(s), statusDotSize, margin)
			name := filepath.Join(dir, "dot-"+s.String()+"-margin"+string(rune('0'+margin))+".png")
			writePNG(t, name, scaleUp(dot, 16))
		}
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// scaleUp blows an image up with nearest neighbours, so a 16px icon can be
// looked at.
func scaleUp(src *image.RGBA, factor int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))

	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			dst.SetRGBA(x, y, src.RGBAAt(b.Min.X+x/factor, b.Min.Y+y/factor))
		}
	}
	return dst
}

var errTest = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
