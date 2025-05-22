package widgets

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/hajimehoshi/guigui"

	"github.com/mjibson/go-dsp/fft"
	"github.com/mjibson/go-dsp/window"
)

type Audiometer struct {
	guigui.DefaultWidget

	numBands   int // number of bars

	bars []float64
}

func (m *Audiometer) Draw(context *guigui.Context, dst *ebiten.Image) {
	bb := context.Bounds(m)
	bx := float32(bb.Min.X)
	by := float32(bb.Min.Y)
	bw := float32(bb.Dx())
	bh := float32(bb.Dy())

	// Fill the background
	vector.DrawFilledRect(dst, bx, by, bw, bh, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased

	w := bw / float32(len(m.bars))
	x := bx

	for i := 0; i < len(m.bars); i++ {
		// Normalize and scale the band value
		h := float32(math.Log10(1+m.bars[i]) * float64(bh))
		if h > bh {
			h = bh
		}

		vector.DrawFilledRect(dst, x, by, w-2, h, color.RGBA{0xe0, 0xe0, 0xe0, 0xe0}, false) // anti-aliased
		x += w
	}
}

func (m *Audiometer) Update(context *guigui.Context, samples []int16, sampleRate int) {
	// Convert int16 samples to float64
	fsamples := make([]float64, len(samples))
	for i := 0; i < len(samples); i++ {
		fsamples[i] = float64(samples[i])
	}

	// Apply Hanning window
	window.Apply(fsamples, window.Hann)

	// Perform FFT
	spectrum := fft.FFTReal(fsamples)

	fres := float64(sampleRate) / float64(len(spectrum))

	if int(fres) < m.numBands || m.numBands == 0 {
		m.numBands = int(fres)
	}

	// Calculate frequency bands
	m.bars = make([]float64, m.numBands)
	binWidth := int(fres) / m.numBands

	for i := 0; i < m.numBands; i++ {
		startBin := i * binWidth
		endBin := (i + 1) * binWidth

		// Sum magnitude of frequencies in this band
		sum := 0.0
		for j := startBin; j < endBin; j++ {
			if j < len(spectrum) {
				sum += math.Sqrt(real(spectrum[j])*real(spectrum[j]) +
					imag(spectrum[j])*imag(spectrum[j]))
			}
		}
		m.bars[i] = sum / float64(binWidth)
	}

	guigui.RequestRedraw(m)
}
