package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

//go:embed winres/icons/gopher32.png
var gopherIconPNG []byte

const trayIconSize = 32

// statusColors maps each guard state to its indicator color.
var statusColors = map[GuardState]color.RGBA{
	StateOK:      {R: 0x22, G: 0xc5, B: 0x5e, A: 0xff}, // green
	StateIdle:    {R: 0xea, G: 0xb3, B: 0x08, A: 0xff}, // yellow
	StateError:   {R: 0xef, G: 0x44, B: 0x44, A: 0xff}, // red
	StateRunning: {R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}, // blue
}

func colorForState(s GuardState) color.RGBA {
	if c, ok := statusColors[s]; ok {
		return c
	}
	return statusColors[StateIdle]
}

// statusDotICO renders a plain circular dot, used next to each guard name in
// the tray menu.
func statusDotICO(state GuardState) []byte {
	return dotICO(state, 16)
}

func dotICO(state GuardState, size int) []byte {
	return encodeICO(dotImage(colorForState(state), size))
}

// trayIconICO renders the app icon with a small state badge in the corner, so
// the tray keeps its identity while still showing the overall state.
func trayIconICO(state GuardState) []byte {
	img := gopherImage(trayIconSize)
	if img == nil {
		return dotICO(state, trayIconSize)
	}
	drawBadge(img, colorForState(state))
	return encodeICO(img)
}

func gopherImage(size int) *image.RGBA {
	src, err := png.Decode(bytes.NewReader(gopherIconPNG))
	if err != nil {
		return nil
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)
	return img
}

// badge geometry, in pixels of the 32x32 tray icon.
const (
	badgeRadius  = 5.5
	badgeRing    = 1.5
	badgeCenterX = 26.0
	badgeCenterY = 26.0
)

var badgeOutline = color.RGBA{R: 0x0f, G: 0x17, B: 0x2a, A: 0xff}

// drawBadge composites an anti-aliased status dot with a dark outline onto the
// top-right corner of the icon.
func drawBadge(img *image.RGBA, c color.RGBA) {
	const ss = 4
	b := img.Bounds()

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			var dotCov, ringCov float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					dx := px - badgeCenterX
					dy := py - badgeCenterY
					d := math.Hypot(dx, dy)
					switch {
					case d <= badgeRadius:
						dotCov++
					case d <= badgeRadius+badgeRing:
						ringCov++
					}
				}
			}
			if dotCov == 0 && ringCov == 0 {
				continue
			}

			n := float64(ss * ss)
			px := img.RGBAAt(x, y)
			if ringCov > 0 {
				px = over(px, badgeOutline, ringCov/n)
			}
			if dotCov > 0 {
				px = over(px, c, dotCov/n)
			}
			img.SetRGBA(x, y, px)
		}
	}
}

// over composites src over dst with straight (non-premultiplied) alpha, where
// cov scales the source alpha for anti-aliasing.
func over(dst, src color.RGBA, cov float64) color.RGBA {
	sa := float64(src.A) / 255 * cov
	da := float64(dst.A) / 255
	oa := sa + da*(1-sa)
	if oa == 0 {
		return color.RGBA{}
	}
	mix := func(s, d uint8) uint8 {
		v := (float64(s)*sa + float64(d)*da*(1-sa)) / oa
		return uint8(v + 0.5)
	}
	return color.RGBA{
		R: mix(src.R, dst.R),
		G: mix(src.G, dst.G),
		B: mix(src.B, dst.B),
		A: uint8(oa*255 + 0.5),
	}
}

// dotImage draws a filled, anti-aliased circle of the given size.
func dotImage(c color.RGBA, size int) *image.RGBA {
	const ss = 4

	big := image.NewRGBA(image.Rect(0, 0, size*ss, size*ss))
	center := float64(size*ss) / 2
	radius := center - float64(ss)
	r2 := radius * radius

	for y := 0; y < size*ss; y++ {
		for x := 0; x < size*ss; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			if dx*dx+dy*dy <= r2 {
				big.SetRGBA(x, y, c)
			}
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var sr, sg, sb, sa uint32
			for j := 0; j < ss; j++ {
				for i := 0; i < ss; i++ {
					p := big.RGBAAt(x*ss+i, y*ss+j)
					sr += uint32(p.R)
					sg += uint32(p.G)
					sb += uint32(p.B)
					sa += uint32(p.A)
				}
			}
			n := uint32(ss * ss)
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(sr / n),
				G: uint8(sg / n),
				B: uint8(sb / n),
				A: uint8(sa / n),
			})
		}
	}
	return dst
}

// encodeICO wraps an image in a single-entry .ico container using a 32-bit
// BGRA bitmap with a fully transparent AND mask (alpha channel carries the
// shape). This is the classic format every Windows version understands.
func encodeICO(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	xor := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := img.RGBAAt(x, y)
			i := ((h-1-y)*w + x) * 4
			xor[i+0] = p.B
			xor[i+1] = p.G
			xor[i+2] = p.R
			xor[i+3] = p.A
		}
	}

	rowBytes := ((w + 31) / 32) * 4
	and := make([]byte, rowBytes*h)

	hdr := make([]byte, 40)
	binary.LittleEndian.PutUint32(hdr[0:], 40)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(h*2))
	binary.LittleEndian.PutUint16(hdr[12:], 1)
	binary.LittleEndian.PutUint16(hdr[14:], 32)

	payload := make([]byte, 0, len(hdr)+len(xor)+len(and))
	payload = append(payload, hdr...)
	payload = append(payload, xor...)
	payload = append(payload, and...)

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	out.WriteByte(byte(w))
	out.WriteByte(byte(h))
	out.WriteByte(0)
	out.WriteByte(0)
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(32))
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(payload)))
	_ = binary.Write(&out, binary.LittleEndian, uint32(22))
	out.Write(payload)

	return out.Bytes()
}
