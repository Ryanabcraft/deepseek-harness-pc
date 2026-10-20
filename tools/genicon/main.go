package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var sizes = []int{256, 128, 64, 48, 32, 16}

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "assets", "deepseek-500.png"))
	if err != nil {
		log.Fatal(err)
	}
	logo, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		log.Fatal(err)
	}

	var frames [][]byte
	for _, s := range sizes {
		img := compose(logo, s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			log.Fatal(err)
		}
		frames = append(frames, buf.Bytes())
	}

	_ = os.WriteFile(filepath.Join(root, "preview.png"), frames[0], 0o644)

	ico := encodeICO(sizes, frames)
	out := filepath.Join(root, "icon.ico")
	if err := os.WriteFile(out, ico, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes)", out, len(ico))
}

// compose paints the whale silhouette white over a blue gradient tile.
func compose(logo image.Image, size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	radius := float64(size) * 0.22

	// tile
	for y := 0; y < size; y++ {
		t := float64(y) / float64(size-1)
		r := uint8(77 + t*(52-77))
		g := uint8(107 + t*(80-107))
		b := uint8(254 + t*(224-254))
		for x := 0; x < size; x++ {
			if insideRounded(float64(x)+0.5, float64(y)+0.5, float64(size), radius) {
				img.SetNRGBA(x, y, color.NRGBA{r, g, b, 255})
			}
		}
	}

	// whale, centered, 68% of the tile
	b := logo.Bounds()
	scale := float64(size) * 0.68 / math.Max(float64(b.Dx()), float64(b.Dy()))
	dw := float64(b.Dx()) * scale
	dh := float64(b.Dy()) * scale
	ox := (float64(size) - dw) / 2
	oy := (float64(size) - dh) / 2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5
			sx := (fx-ox)/scale + float64(b.Min.X)
			sy := (fy-oy)/scale + float64(b.Min.Y)
			a := sampleAlpha(logo, sx, sy)
			if a <= 0.004 {
				continue
			}
			if !insideRounded(fx, fy, float64(size), radius) {
				continue
			}
			dst := img.NRGBAAt(x, y)
			na := a
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(float64(dst.R)*(1-na) + 255*na),
				G: uint8(float64(dst.G)*(1-na) + 255*na),
				B: uint8(float64(dst.B)*(1-na) + 255*na),
				A: 255,
			})
		}
	}
	return img
}

func insideRounded(x, y, size, radius float64) bool {
	if x < 0 || y < 0 || x > size || y > size {
		return false
	}
	cx := math.Min(math.Max(x, radius), size-radius)
	cy := math.Min(math.Max(y, radius), size-radius)
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= radius*radius
}

// sampleAlpha does bilinear sampling of the source's alpha channel.
func sampleAlpha(src image.Image, x, y float64) float64 {
	b := src.Bounds()
	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	fx := x - float64(x0)
	fy := y - float64(y0)

	at := func(px, py int) float64 {
		if px < b.Min.X || py < b.Min.Y || px >= b.Max.X || py >= b.Max.Y {
			return 0
		}
		_, _, _, a := src.At(px, py).RGBA()
		return float64(a) / 65535.0
	}
	return at(x0, y0)*(1-fx)*(1-fy) +
		at(x0+1, y0)*fx*(1-fy) +
		at(x0, y0+1)*(1-fx)*fy +
		at(x0+1, y0+1)*fx*fy
}

// encodeICO writes a PNG-compressed ICO (supported since Windows Vista).
func encodeICO(sizes []int, frames [][]byte) []byte {
	var out bytes.Buffer
	header := make([]byte, 6)
	binary.LittleEndian.PutUint16(header[2:], 1)
	binary.LittleEndian.PutUint16(header[4:], uint16(len(frames)))
	out.Write(header)

	offset := 6 + 16*len(frames)
	for i, frame := range frames {
		s := sizes[i]
		entry := make([]byte, 16)
		if s >= 256 {
			entry[0] = 0
			entry[1] = 0
		} else {
			entry[0] = byte(s)
			entry[1] = byte(s)
		}
		binary.LittleEndian.PutUint16(entry[4:], 1)
		binary.LittleEndian.PutUint16(entry[6:], 32)
		binary.LittleEndian.PutUint32(entry[8:], uint32(len(frame)))
		binary.LittleEndian.PutUint32(entry[12:], uint32(offset))
		out.Write(entry)
		offset += len(frame)
	}
	for _, frame := range frames {
		out.Write(frame)
	}
	return out.Bytes()
}
