package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const fontSize = 28

var (
	whiteImage = ebiten.NewImage(3, 3)

	// whiteSubImage is an internal sub image of whiteImage.
	// Use whiteSubImage at DrawTriangles instead of whiteImage in order to avoid bleeding edges.
	whiteSubImage = whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)

	fface text.Face
)

func init() {
	whiteImage.Fill(color.White)

	ff, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		log.Fatal("error loading font", err)
	}

	fface = &text.GoTextFace{Source: ff, Size: 28}
}

const (
	screenWidth  = 640
	screenHeight = 480
)

func maxCounter(index int) int {
	return 128 + (17*index+32)%64
}

type Game struct {
	vertices []ebiten.Vertex
	indices  []uint16

	samples [1920]int16

	numberInput *NumberInput
}

type NumberInput struct {
	value       int
	minValue    int
	maxValue    int
	maxDigits   int
	position    image.Point
	ddimensions image.Point
	focused     bool
	editing     bool
	cursor      int
}

func NewNumberInput(minValue, maxValue int, x, y int) *NumberInput {
	maxDigits := 0
	for i := 1; i <= maxValue; i *= 10 {
		maxDigits++
	}

	dw, dh := text.Measure("0 ", fface, 0)
	m := fface.Metrics()
	dh = m.HLineGap + m.HAscent + m.HDescent + m.HLineGap

	return &NumberInput{
		value:       minValue,
		minValue:    minValue,
		maxValue:    maxValue,
		maxDigits:   maxDigits,
		position:    image.Point{x, y},
		ddimensions: image.Point{int(dw), int(dh + 0)},
	}
}

func (n *NumberInput) Size() (int, int) {
	return n.ddimensions.X * n.maxDigits, n.ddimensions.Y
}

func (n *NumberInput) Update() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		mousePos := image.Point{x, y}
		// Check if click is within number input bounds
		bounds := image.Rect(n.position.X, n.position.Y, n.position.X+n.maxDigits*n.ddimensions.X, n.position.Y+n.ddimensions.Y)
		n.focused = mousePos.In(bounds)
		if n.focused {
			n.editing = true
			// Calculate which digit was clicked
			n.cursor = (x - n.position.X) / n.ddimensions.X
			if n.cursor >= n.maxDigits {
				n.cursor = n.maxDigits - 1
			}
		}
	}

	if n.focused {
		_, dy := ebiten.Wheel()
		if dy != 0 {
			// Calculate the multiplier based on cursor position
			multiplier := 1
			for i := 0; i < n.maxDigits-1-n.cursor; i++ {
				multiplier *= 10
			}
			if dy < 0 {
				n.value += multiplier
			} else {
				n.value -= multiplier
			}
			// Ensure value stays positive and within bounds
			if n.value < n.minValue {
				n.value = n.minValue
			}
			if n.value >= n.maxValue {
				n.value = n.maxValue
			}
		}

		if n.editing {
			// Handle numeric input
			for k := ebiten.Key0; k <= ebiten.Key9; k++ {
				if inpututil.IsKeyJustPressed(k) {
					digit := int(k) - int(ebiten.Key0)
					// Calculate position value based on cursor
					multiplier := 1
					for i := 0; i < n.maxDigits-1-n.cursor; i++ {
						multiplier *= 10
					}
					// Clear the digit at cursor position and set new value
					n.value = (n.value/multiplier/10)*multiplier*10 + digit*multiplier + n.value%multiplier
					if n.value < n.minValue {
						n.value = n.minValue
					}
					if n.value >= n.maxValue {
						n.value = n.maxValue
					}

					n.cursor++
					if n.cursor >= n.maxDigits {
						n.editing = false
					}
				}
			}

			// Handle backspace and arrow keys
			if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || inpututil.IsKeyJustPressed(ebiten.KeyDelete) || inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
				if n.cursor > 0 {
					n.cursor--
				}
			} else if inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeyTab) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
				if n.cursor < n.maxDigits {
					n.cursor++
				}
			}
		}
	}
}

func (n *NumberInput) Draw(screen *ebiten.Image) {
	// Draw background
	vector.DrawFilledRect(screen, float32(n.position.X), float32(n.position.Y), float32(n.maxDigits*n.ddimensions.X), float32(n.ddimensions.Y), color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased

	// Draw digits
	str := fmt.Sprintf(fmt.Sprintf("%%0%dd", n.maxDigits), n.value)
	for i, ch := range str {
		x := n.position.X + i*n.ddimensions.X
		//ebitenutil.DebugPrintAt(screen, string(ch), x+5, n.position.Y+8)

		op := &text.DrawOptions{}
		op.GeoM.Translate(float64(x+4), float64(n.position.Y+4))
		op.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, string(ch), fface, op)
	}

	// Draw cursor if editing
	if n.editing && n.cursor < n.maxDigits {
		x := n.position.X + n.cursor*n.ddimensions.X
		vector.DrawFilledRect(screen, float32(x), float32(n.position.Y+n.ddimensions.Y-4), float32(n.ddimensions.X), 2, color.RGBA{0xff, 0xff, 0xff, 0xff}, false) // anti-aliased
	}
}

func (g *Game) drawWave(screen *ebiten.Image, bounds image.Rectangle, samples []int16) {
	var path vector.Path

	wx, wy := float32(bounds.Min.X), float32(bounds.Min.Y)
	ww, wh := float32(bounds.Dx()), float32(bounds.Dy())

	npoints := len(samples)
	indexToPoint := func(i int, v int16) (float32, float32) {
		x := wx + float32(i*int(ww)/npoints)
		// Center the wave vertically and scale the amplitude
		y := wy + (float32(v) / 32768.0 * float32(wh/2))
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
	g.vertices, g.indices = path.AppendVerticesAndIndicesForStroke(g.vertices[:0], g.indices[:0], &vector.StrokeOptions{
		Width:        2,
		LineJoin:     vector.LineJoinRound,
		LineCap:      vector.LineCapRound,
	})

	// Set color for the wave
	for i := range g.vertices {
		g.vertices[i].SrcX = 1
		g.vertices[i].SrcY = 1
		g.vertices[i].ColorR = 0x33 / float32(0xff)
		g.vertices[i].ColorG = 0x66 / float32(0xff)
		g.vertices[i].ColorB = 0xff / float32(0xff)
		g.vertices[i].ColorA = 1
	}

	screen.DrawTriangles(g.vertices, g.indices, whiteSubImage, &ebiten.DrawTrianglesOptions{
		FillRule: ebiten.FillRuleNonZero,
	})
}

func (g *Game) Draw(screen *ebiten.Image) {
	dst := screen

	dst.Fill(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})

	w, h := g.numberInput.Size()
	p := image.Rect(100, 200, 100+w, 200+h)
	g.drawWave(dst, p, g.samples[:])

	g.numberInput.Draw(screen)

	msg := fmt.Sprintf("TPS: %0.2f\nFPS: %0.2f", ebiten.ActualTPS(), ebiten.ActualFPS())
	msg += "\nPress A to switch anti-alias."
	msg += "\nPress L to switch the fill mode and the line mode."
	ebitenutil.DebugPrint(screen, msg)
}

func randomInt16(rmin, rmax int16) int16 {
	return rmin + int16(rand.Intn(int(rmax-rmin)))
}

func (g *Game) Update() error {
	g.numberInput.Update()

	for i := 0; i < len(g.samples); i++ {
		g.samples[i] = randomInt16(-16000, 16000)
	}

	return nil
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	g := &Game{}
	g.numberInput = NewNumberInput(137000000, 174000000, 100, 100)

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Audio wave")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
