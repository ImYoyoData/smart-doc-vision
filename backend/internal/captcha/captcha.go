package captcha

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	mrand "math/rand"
	"time"
)

const (
	Width     = 320
	Height    = 160
	PieceSize = 46
	padding   = 22
)

type Challenge struct {
	Background []byte
	Piece      []byte
	Width      int
	Height     int
	PieceSize  int
	PieceY     int
	ExpiresIn  int
}

// Generate creates a slider-jigsaw challenge and returns the horizontal
// coordinate of the missing piece. The answer is intentionally not part of
// Challenge and must be stored server-side.
func Generate() (*Challenge, int, error) {
	rng := newRand()
	base := newBackground(rng, Width, Height)

	minX := padding
	maxX := Width - PieceSize - padding
	answerX := minX + rng.Intn(maxX-minX+1)
	minY := 30
	maxY := Height - PieceSize - 24
	answerY := minY + rng.Intn(maxY-minY+1)

	piece := cutPiece(base, answerX, answerY, PieceSize)
	drawHole(base, answerX, answerY, PieceSize)

	var bg bytes.Buffer
	if err := jpeg.Encode(&bg, base, &jpeg.Options{Quality: 90}); err != nil {
		return nil, 0, err
	}
	var pieceBuf bytes.Buffer
	if err := png.Encode(&pieceBuf, piece); err != nil {
		return nil, 0, err
	}
	return &Challenge{
		Background: bg.Bytes(),
		Piece:      pieceBuf.Bytes(),
		Width:      Width,
		Height:     Height,
		PieceSize:  PieceSize,
		PieceY:     answerY,
		ExpiresIn:  120,
	}, answerX, nil
}

func newRand() *mrand.Rand {
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err == nil {
		return mrand.New(mrand.NewSource(int64(binary.LittleEndian.Uint64(seedBytes[:]))))
	}
	return mrand.New(mrand.NewSource(time.Now().UnixNano()))
}

func newBackground(rng *mrand.Rand, width, height int) *image.RGBA {
	palettes := [][2][3]uint8{
		{{16, 42, 92}, {39, 90, 185}},
		{{35, 30, 92}, {91, 62, 170}},
		{{10, 70, 88}, {35, 136, 138}},
		{{65, 30, 86}, {146, 62, 125}},
		{{28, 54, 72}, {76, 118, 158}},
	}
	palette := palettes[rng.Intn(len(palettes))]
	c1, c2 := palette[0], palette[1]
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			t := float64(x)/float64(width)*0.72 + float64(y)/float64(height)*0.28
			r := uint8(float64(c1[0])*(1-t) + float64(c2[0])*t)
			g := uint8(float64(c1[1])*(1-t) + float64(c2[1])*t)
			b := uint8(float64(c1[2])*(1-t) + float64(c2[2])*t)
			noise := rng.Intn(13) - 6
			img.SetRGBA(x, y, color.RGBA{
				R: clampByte(int(r) + noise),
				G: clampByte(int(g) + noise),
				B: clampByte(int(b) + noise),
				A: 255,
			})
		}
	}

	// Soft translucent document card.
	draw.Draw(img, image.Rect(34, 24, width-34, height-24), &image.Uniform{
		C: color.RGBA{R: 255, G: 255, B: 255, A: 26},
	}, image.Point{}, draw.Over)
	lineColor := color.RGBA{R: 255, G: 255, B: 255, A: 45}
	for i := 0; i < 5; i++ {
		y := 46 + i*21
		drawLine(img, 58, y, width-58-rng.Intn(60), y, lineColor, 2)
	}

	for i := 0; i < 6; i++ {
		cx := 20 + rng.Intn(width-40)
		cy := 15 + rng.Intn(height-30)
		radius := 12 + rng.Intn(30)
		fillCircle(img, cx, cy, radius, color.RGBA{
			R: uint8(90 + rng.Intn(120)), G: uint8(120 + rng.Intn(100)),
			B: 255, A: uint8(10 + rng.Intn(20)),
		})
	}
	for i := 0; i < 9; i++ {
		drawLine(img, rng.Intn(width), rng.Intn(height), rng.Intn(width), rng.Intn(height), color.RGBA{
			R: 255, G: 255, B: 255, A: uint8(8 + rng.Intn(14)),
		}, 1+rng.Intn(2))
	}
	for i := 0; i < 40; i++ {
		x := rng.Intn(width)
		y := rng.Intn(height)
		img.SetRGBA(x, y, blend(img.RGBAAt(x, y), color.RGBA{R: 255, G: 255, B: 255, A: 70}))
	}
	return img
}

func cutPiece(base *image.RGBA, x0, y0, size int) *image.RGBA {
	piece := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !insidePiece(x, y, size) {
				continue
			}
			c := base.RGBAAt(x0+x, y0+y)
			if pieceEdge(x, y, size) {
				c = blend(c, color.RGBA{R: 248, G: 251, B: 255, A: 200})
			}
			piece.SetRGBA(x, y, c)
		}
	}
	return piece
}

func drawHole(base *image.RGBA, x0, y0, size int) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !insidePiece(x, y, size) {
				continue
			}
			px, py := x0+x, y0+y
			if pieceEdge(x, y, size) {
				base.SetRGBA(px, py, color.RGBA{R: 238, G: 245, B: 255, A: 255})
				continue
			}
			c := base.RGBAAt(px, py)
			base.SetRGBA(px, py, color.RGBA{
				R: uint8(float64(c.R) * 0.38),
				G: uint8(float64(c.G) * 0.38),
				B: uint8(float64(c.B) * 0.38),
				A: 255,
			})
		}
	}
}

func insidePiece(x, y, size int) bool {
	if x < 0 || y < 0 || x >= size || y >= size {
		return false
	}
	const radius = 7.0
	fx, fy := float64(x)+0.5, float64(y)+0.5
	corners := [][2]float64{
		{radius, radius},
		{float64(size) - radius, radius},
		{radius, float64(size) - radius},
		{float64(size) - radius, float64(size) - radius},
	}
	for _, corner := range corners {
		nearX := (corner[0] == radius && fx < radius) || (corner[0] != radius && fx > float64(size)-radius)
		nearY := (corner[1] == radius && fy < radius) || (corner[1] != radius && fy > float64(size)-radius)
		if nearX && nearY {
			dx, dy := fx-corner[0], fy-corner[1]
			if dx*dx+dy*dy > radius*radius {
				return false
			}
		}
	}
	// A semicircular bite on the right edge makes the puzzle piece easy to
	// recognize while still requiring accurate alignment.
	dx := fx - float64(size-2)
	dy := fy - float64(size)/2
	if dx*dx+dy*dy < 8.5*8.5 {
		return false
	}
	return true
}

func pieceEdge(x, y, size int) bool {
	if !insidePiece(x, y, size) {
		return false
	}
	return !insidePiece(x-1, y, size) || !insidePiece(x+1, y, size) ||
		!insidePiece(x, y-1, size) || !insidePiece(x, y+1, size)
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, width int) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx, sy := -1, -1
	if x0 < x1 {
		sx = 1
	}
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		fillCircle(img, x0, y0, width, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, radius int, c color.RGBA) {
	if radius < 1 {
		return
	}
	for y := cy - radius; y <= cy+radius; y++ {
		if y < 0 || y >= img.Bounds().Dy() {
			continue
		}
		for x := cx - radius; x <= cx+radius; x++ {
			if x < 0 || x >= img.Bounds().Dx() {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= radius*radius {
				img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c))
			}
		}
	}
}

func blend(dst, src color.RGBA) color.RGBA {
	alpha := float64(src.A) / 255.0
	return color.RGBA{
		R: clampByte(int(float64(src.R)*alpha + float64(dst.R)*(1-alpha))),
		G: clampByte(int(float64(src.G)*alpha + float64(dst.G)*(1-alpha))),
		B: clampByte(int(float64(src.B)*alpha + float64(dst.B)*(1-alpha))),
		A: 255,
	}
}

func clampByte(value int) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}
