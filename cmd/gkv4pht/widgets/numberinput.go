package widgets

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/hajimehoshi/guigui"
	"github.com/hajimehoshi/guigui/basicwidget"
)

const (
	scale = 1.0
)

var (
	numberFont *text.GoTextFaceSource
)

func init() {
	ff, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		log.Fatal("error loading font", err)
	}

	numberFont = ff
}

func getFontAndMetrics(context *guigui.Context) (font *text.GoTextFace, dw, dh float32) {
	font = &text.GoTextFace{Source: numberFont, Size: basicwidget.FontSize(context) * 2}

	m := font.Metrics()
	dw = float32(text.Advance("0 ", font))
	dh = float32(m.HLineGap + m.HAscent + m.HDescent + m.HLineGap)
	return
}

type NumberInput struct {
	guigui.DefaultWidget

	value     int
	minValue  int
	maxValue  int
	maxDigits int
	separator rune
	step      int
	editing   bool
	cursor    int

	onValueChanged func(value int)
}

func (n *NumberInput) DefaultSize(context *guigui.Context) image.Point {
	// Calculate the size based on the number of digits
	_, dw, dh := getFontAndMetrics(context)
	return image.Pt(int(dw*scale)*(n.maxDigits+2), int(dh*scale))
}

func (n *NumberInput) SetSeparator(sep rune) {
	n.separator = sep
}

func (n *NumberInput) SetStep(step int) {
	n.step = step
}

func (n *NumberInput) SetLimits(minValue, maxValue int) {
	n.minValue = minValue
	n.maxValue = maxValue

	if n.minValue > n.maxValue {
		n.minValue, n.maxValue = n.maxValue, n.minValue
	}
	if n.minValue < 0 {
		n.minValue = 0
	}
	if n.value < minValue {
		n.value = minValue
	}
	if n.value > maxValue {
		n.value = maxValue
	}

	n.maxDigits = 0
	for i := 1; i <= maxValue; i *= 10 {
		n.maxDigits++
	}

	guigui.RequestRedraw(n)
}

func (n *NumberInput) Value() int {
	return n.value
}

func (n *NumberInput) SetValue(value int) {
	if value < n.minValue {
		value = n.minValue
	}
	if value > n.maxValue {
		value = n.maxValue
	}
	n.value = value
	n.cursor = 0
	n.editing = false

	guigui.RequestRedraw(n)
}

func (n *NumberInput) SetOnValueChanged(callback func(value int)) {
	n.onValueChanged = callback
}

func (n *NumberInput) HandlePointingInput(context *guigui.Context) guigui.HandleInputResult {
	if !context.IsFocusedOrHasFocusedChild(n) {
		n.cursor = n.maxDigits
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		b := context.Bounds(n)
		c := image.Pt(ebiten.CursorPosition()).Sub(b.Min)

		_, dw, _ := getFontAndMetrics(context)

		// Calculate which digit was clicked
		cursor := c.X / int(dw*scale)
		if cursor < 0 {
			cursor = 0
		} else if cursor >= n.maxDigits {
			cursor = n.maxDigits - 1
		}

		if cursor != n.cursor {
			n.editing = true
			n.cursor = cursor
			context.SetFocused(n, true)
			guigui.RequestRedraw(n)
		}
	} else if n.cursor >= n.maxDigits {
		guigui.RequestRedraw(n)
		n.editing = false
	}

	return guigui.HandleInputResult{}
}

func (n *NumberInput) HandleButtonInput(context *guigui.Context) guigui.HandleInputResult {
	prev := n.value
	prevCursor := n.cursor

	_, dy := ebiten.Wheel()
	if dy != 0 {
		// Calculate the multiplier based on cursor position
		multiplier := 1
		for i := 0; i < n.maxDigits-1-n.cursor; i++ {
			multiplier *= 10
		}
		if dy < 0 {
			n.value += multiplier

			if n.step != 0 && n.value%n.step != 0 {
				n.value += n.step - (n.value % n.step)
			}
		} else {
			n.value -= multiplier

			if n.step != 0 && n.value%n.step != 0 {
				n.value -= (n.value % n.step)
			}
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
		} else if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
			// Calculate the multiplier based on cursor position
			multiplier := 1
			for i := 0; i < n.maxDigits-1-n.cursor; i++ {
				multiplier *= 10
			}
			n.value += multiplier

			if n.step != 0 && n.value%n.step != 0 {
				n.value += n.step - (n.value % n.step)
			}
		} else if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
			// Calculate the multiplier based on cursor position
			multiplier := 1
			for i := 0; i < n.maxDigits-1-n.cursor; i++ {
				multiplier *= 10
			}
			n.value -= multiplier

			if n.step != 0 && n.value%n.step != 0 {
				n.value -= (n.value % n.step)
			}
		}

	}

	// Ensure value is within limits
	if n.value < n.minValue {
		n.value = n.minValue
	} else if n.value >= n.maxValue {
		n.value = n.maxValue
	}

	if n.value != prev && n.onValueChanged != nil {
		n.onValueChanged(n.value)
	}

	if n.value != prev || n.cursor != prevCursor {
		guigui.RequestRedraw(n)
	}

	return guigui.HandleInputResult{}
}

func (n *NumberInput) Draw(context *guigui.Context, dst *ebiten.Image) {
	b := context.Bounds(n)
	bx := float32(b.Min.X)
	by := float32(b.Min.Y)

	font, dw, dh := getFontAndMetrics(context)
	sw := dw * scale
	sh := dh * scale

	// Draw background
	vector.DrawFilledRect(dst, bx, by, float32(n.maxDigits)*sw, sh, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased

	// Draw digits
	str := fmt.Sprintf(fmt.Sprintf("%%0%dd", n.maxDigits), n.value)
	for i, ch := range str {
		x := int(bx) + i*int(sw)

		digit := string(ch)
		if n.separator != 0 && (n.maxDigits-i)%3 == 1 && i != n.maxDigits-1 {
			digit += string(n.separator)
		}

		op := &text.DrawOptions{}
		op.GeoM.Scale(float64(scale), float64(scale))
		op.GeoM.Translate(float64(x+8), float64(by+4))
		op.ColorScale.ScaleWithColor(color.White)
		text.Draw(dst, digit, font, op)
	}

	// Draw cursor if editing
	if n.editing && n.cursor < n.maxDigits {
		x := int(bx) + int(float32(n.cursor)*sw)
		vector.DrawFilledRect(dst, float32(x), by+sh-4, sw, 2, color.RGBA{0xff, 0xff, 0xff, 0xff}, false) // anti-aliased
	}
}
