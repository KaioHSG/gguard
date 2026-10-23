package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// statusColors maps each guard state to the tray dot color.
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

// statusDotICO renders an anti-aliased circular dot for the given state as a
// Windows .ico byte slice.
func statusDotICO(s GuardState) []byte {
	return encodeICO(dotImage(colorForState(s), 32))
}

// dotImage draws a filled, anti-aliased circle of the given size.
func dotImage(c color.RGBA, size int) *image.RGBA {
	const ss = 4 // supersampling factor

	big := image.NewRGBA(image.Rect(0, 0, size*ss, size*ss))
	center := float64(size*ss) / 2
	radius := center - float64(ss) // small margin so the dot is not clipped
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

	// XOR bitmap: 32-bit BGRA, rows bottom-up.
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

	// AND mask: 1bpp, each row padded to a 4-byte boundary.
	rowBytes := ((w + 31) / 32) * 4
	and := make([]byte, rowBytes*h)

	// BITMAPINFOHEADER (height is doubled to cover XOR + AND masks).
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
	// ICONDIR
	_ = binary.Write(&out, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // type: icon
	_ = binary.Write(&out, binary.LittleEndian, uint16(1)) // count
	// ICONDIRENTRY
	out.WriteByte(byte(w))                                            // width (0 would mean 256)
	out.WriteByte(byte(h))                                            // height
	out.WriteByte(0)                                                  // palette colors
	out.WriteByte(0)                                                  // reserved
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))            // planes
	_ = binary.Write(&out, binary.LittleEndian, uint16(32))           // bits per pixel
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(payload))) // size
	_ = binary.Write(&out, binary.LittleEndian, uint32(22))           // offset
	out.Write(payload)

	return out.Bytes()
}
