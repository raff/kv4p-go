package widgets

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/hajimehoshi/guigui"
)

type Bar struct {
	guigui.DefaultWidget
	minValue, maxValue, value int
}

func (b *Bar) Draw(context *guigui.Context, dst *ebiten.Image) {
	// Update the waveform data

	bb := context.Bounds(b)
	bx := float32(bb.Min.X)
	by := float32(bb.Min.Y)
	bw := float32(bb.Dx())
	bh := float32(bb.Dy())

	w := bw / float32(b.maxValue - b.minValue)

	// Draw the S-meter
	vector.DrawFilledRect(dst, bx, by, bw, bh, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased
	if b.value == 0 {
		return
	}

	// Draw the S-meter scale
	for i := 0; i < b.value; i++ {
		x := bx + float32(i)*w
		vector.DrawFilledRect(dst, x+4, by+4, w-4, bh-8, color.RGBA{0xe0, 0xe0, 0xe0, 0xe0}, false) // anti-aliased
	}
}

func (b *Bar) SetLimits(minValue, maxValue int) {
	if minValue < 0 {
		minValue = 0
	}
	if maxValue > 100 {
		maxValue = 100
	}
	if minValue >= maxValue {
		minValue = 0
		maxValue = 100
	}

	b.minValue = minValue
	b.maxValue = maxValue
}

func (b *Bar) SetValue(value int) {
	if b.maxValue == 0 {
		b.maxValue = 100
	}

	if value < b.minValue {
		value = b.minValue
	} else if value > b.maxValue {
		value = b.maxValue
	}

	if value == b.value {
		return
	}

	b.value = value
	guigui.RequestRedraw(b)
}
