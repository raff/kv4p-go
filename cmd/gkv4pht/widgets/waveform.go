package widgets

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/hajimehoshi/guigui"
)

var (
	whiteImage    = ebiten.NewImage(3, 3)
	whiteSubImage = whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
)

func init() {
	whiteImage.Fill(color.White)
}

type Waveform struct {
	guigui.DefaultWidget

	vertices []ebiten.Vertex
	indices  []uint16
}

func (w *Waveform) Draw(context *guigui.Context, dst *ebiten.Image) {
	b := context.Bounds(w)

	// Fill the background
	vector.DrawFilledRect(dst, float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()),
		color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased
	// Draw the waveform
	dst.DrawTriangles(w.vertices, w.indices, whiteSubImage, &ebiten.DrawTrianglesOptions{
		FillRule: ebiten.FillRuleNonZero,
	})
}

func (w *Waveform) Update(context *guigui.Context, samples []int16) {
	// Update the waveform data

	var path vector.Path

	b := context.Bounds(w)
	bx := float32(b.Min.X)
	by := float32(b.Min.Y)
	bw := float32(b.Dx())
	bh := float32(b.Dy())

	npoints := len(samples)
	indexToPoint := func(i int, v int16) (float32, float32) {
		x := bx + float32(i*int(bw)/npoints)
		// Center the wave vertically and scale the amplitude
		y := by + bh/2 + (float32(v) / 32768.0 * bh)
		return x, y
	}

	// Start the path at the first point
	if npoints > 0 {
		x, y := indexToPoint(0, samples[0])
		path.MoveTo(x, y)

		// Draw lines between points
		for i := 1; i < npoints; i++ {
			x, y := indexToPoint(i, samples[i])
			path.LineTo(x, y)
		}
	}

	// Draw just the wave line
	w.vertices, w.indices = path.AppendVerticesAndIndicesForStroke(w.vertices[:0], w.indices[:0], &vector.StrokeOptions{
		Width:    2,
		LineJoin: vector.LineJoinRound,
		LineCap:  vector.LineCapRound,
	})

	// Set color for the wave
	for i := range w.vertices {
		w.vertices[i].SrcX = 1
		w.vertices[i].SrcY = 1
		w.vertices[i].ColorR = 0x33 / float32(0xff)
		w.vertices[i].ColorG = 0xee / float32(0xff)
		w.vertices[i].ColorB = 0x00 / float32(0xff)
		w.vertices[i].ColorA = 1
	}

	guigui.RequestRedraw(w)
}
