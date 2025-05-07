package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/raff/kv4p-go"
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
	samples []int16

	numberInput *NumberInput
	waveform    *Waveform
	smeter      *SMeter
	quit        bool

	radio   *kv4pht.CommandProcessor
	bw      int
	squelch int
	freq    float64

	smeterValue int
}

type NumberInput struct {
	x, y   float32
	dw, dh float32

	bounds image.Rectangle

	value     int
	minValue  int
	maxValue  int
	maxDigits int
	focused   bool
	editing   bool
	cursor    int

	ValueCallback func(value int)
}

func NewNumberInput(minValue, maxValue int, x, y float32) *NumberInput {
	maxDigits := 0
	for i := 1; i <= maxValue; i *= 10 {
		maxDigits++
	}

	dw, dh := text.Measure("0 ", fface, 0)
	m := fface.Metrics()
	dh = m.HLineGap + m.HAscent + m.HDescent + m.HLineGap

	return &NumberInput{
		x:  x,
		y:  y,
		dw: float32(dw),
		dh: float32(dh),

		bounds: image.Rect(int(x), int(y), int(x)+maxDigits*int(dw), int(y)+int(dh)),

		value:     minValue,
		minValue:  minValue,
		maxValue:  maxValue,
		maxDigits: maxDigits,
	}
}

func (n *NumberInput) Size() (float32, float32) {
	//return n.dw * float32(n.maxDigits), n.dh
	return float32(n.bounds.Dx()), float32(n.bounds.Dy())
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
	n.focused = false
}

func (n *NumberInput) Update() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		mousePos := image.Point{x, y}
		// Check if click is within number input bounds
		n.focused = mousePos.In(n.bounds)
		if n.focused {
			n.editing = true
			// Calculate which digit was clicked
			n.cursor = (x - int(n.x)) / int(n.dw)
			if n.cursor >= n.maxDigits {
				n.cursor = n.maxDigits - 1
			}
		}
	}

	if n.focused {
		prev := n.value

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
			} else if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
				// Calculate the multiplier based on cursor position
				multiplier := 1
				for i := 0; i < n.maxDigits-1-n.cursor; i++ {
					multiplier *= 10
				}
				n.value += multiplier
				// Ensure value stays positive and within bounds
				if n.value >= n.maxValue {
					n.value = n.maxValue
				}
			} else if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
				// Calculate the multiplier based on cursor position
				multiplier := 1
				for i := 0; i < n.maxDigits-1-n.cursor; i++ {
					multiplier *= 10
				}
				n.value -= multiplier
				// Ensure value stays positive and within bounds
				if n.value < n.minValue {
					n.value = n.minValue
				}
			}
		}

		if n.value != prev && n.ValueCallback != nil {
			n.ValueCallback(n.value)
		}
	}
}

func (n *NumberInput) Draw(screen *ebiten.Image) {
	// Draw background
	vector.DrawFilledRect(screen, n.x, n.y, float32(n.maxDigits)*n.dw, n.dh, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased

	// Draw digits
	str := fmt.Sprintf(fmt.Sprintf("%%0%dd", n.maxDigits), n.value)
	for i, ch := range str {
		x := int(n.x) + i*int(n.dw)

		op := &text.DrawOptions{}
		op.GeoM.Translate(float64(x+4), float64(n.y+4))
		op.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, string(ch), fface, op)
	}

	// Draw cursor if editing
	if n.editing && n.cursor < n.maxDigits {
		x := int(n.x) + n.cursor*int(n.dw)
		vector.DrawFilledRect(screen, float32(x), n.y+n.dh-4, n.dw, 2, color.RGBA{0xff, 0xff, 0xff, 0xff}, false) // anti-aliased
	}
}

type Waveform struct {
	vertices []ebiten.Vertex
	indices  []uint16

	x, y, w, h float32
}

func NewWaveform(x, y, w, h float32) *Waveform {
	return &Waveform{
		x: x, y: y, w: w, h: h,
	}
}

func (w *Waveform) Draw(screen *ebiten.Image) {
	// Draw the waveform
	vector.DrawFilledRect(screen, w.x, w.y, w.w, w.h, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased

	screen.DrawTriangles(w.vertices, w.indices, whiteSubImage, &ebiten.DrawTrianglesOptions{
		FillRule: ebiten.FillRuleNonZero,
	})
}

func (w *Waveform) Update(samples []int16) {
	// Update the waveform data

	var path vector.Path

	npoints := len(samples)
	indexToPoint := func(i int, v int16) (float32, float32) {
		x := w.x + float32(i*int(w.w)/npoints)
		// Center the wave vertically and scale the amplitude
		y := w.y + w.h/2 + (float32(v) / 32768.0 * w.h)
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
		w.vertices[i].ColorG = 0x66 / float32(0xff)
		w.vertices[i].ColorB = 0xff / float32(0xff)
		w.vertices[i].ColorA = 1
	}
}

type SMeter struct {
	x, y, w, h float32 // position
	value      int     // S-meter value
}

func NewSMeter(x, y, w, h float32) *SMeter {
	return &SMeter{
		x: x, y: y, w: w, h: h,
	}
}

func (s *SMeter) Draw(screen *ebiten.Image) {
	// Draw the S-meter
	vector.DrawFilledRect(screen, s.x, s.y, s.w, s.h, color.RGBA{0x33, 0x33, 0x33, 0xff}, false) // anti-aliased
	if s.value == 0 {
		return
	}

	w := s.w / 9

	// Draw the S-meter scale
	for i := 0; i < s.value; i++ {
		x := s.x + float32(i)*w
		vector.DrawFilledRect(screen, x+4, s.y+4, w-4, s.h-8, color.RGBA{0xe0, 0xe0, 0xe0, 0xe0}, false) // anti-aliased
	}
}

func (s *SMeter) Update(value int) {
	// Update the S-meter value
	s.value = value
	// Clamp the value to the range 0-9
	if s.value < 1 {
		s.value = 1
	}
	if s.value > 9 {
		s.value = 9
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	dst := screen
	dst.Fill(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})

	g.numberInput.Draw(screen)
	g.waveform.Draw(screen)
	g.smeter.Draw(screen)
}

func randomInt16(rmin, rmax int16) int16 {
	return rmin + int16(rand.Intn(int(rmax-rmin)))
}

func (g *Game) Update() error {
	if g.quit {
		return ebiten.Termination
	}

	g.numberInput.Update()

	for i := 0; i < len(g.samples); i++ {
		g.samples[i] = randomInt16(-16000, 16000)
	}

	g.waveform.Update(g.samples[:])

	g.smeter.Update(g.smeterValue)
	return nil
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	flag.BoolVar(&kv4pht.Debug, "debug", kv4pht.Debug, "Enable debug output")

	band := flag.String("band", "vhf", "Band (vhf, uhf)")
	bw := flag.String("bw", "wide", "Bandwidth (wide=25k, narrow=12.5k)")
	freq := flag.Float64("freq", 162.4, "Frequency in MHz") // NOAA Weather Radio
	squelch := flag.Int("squelch", 0, "Squelch level (0-100)")
	pre := flag.Bool("pre", true, "pre-emphasis filter")
	high := flag.Bool("high", true, "high-pass filter")
	low := flag.Bool("low", true, "low-pass filter")
	reset := flag.Bool("reset", false, "reset board")
	flag.Parse()

	g := &Game{}

	left := float32(20)
	top := float32(20)

	minfreq := int(kv4pht.VHF_MIN_FREQ * 1000000)
	maxfreq := int(kv4pht.VHF_MAX_FREQ * 1000000)
	if *band == "uhf" || *freq >= kv4pht.UHF_MIN_FREQ {
		minfreq = int(kv4pht.UHF_MIN_FREQ * 1000000)
		maxfreq = int(kv4pht.UHF_MAX_FREQ * 1000000)
	}

	g.numberInput = NewNumberInput(minfreq, maxfreq, left, top)
	w, h := g.numberInput.Size()

	top += h + 10
	g.smeter = NewSMeter(left, top, w, h/2)

	top += h/2 + 10
	g.waveform = NewWaveform(left, top, w, h)

	radio, err := kv4pht.Start(*dev)
	if err != nil {
		log.Fatalf("Start: %v", err)
	}

	g.radio = radio
	g.radio.AudioCallback = func(samples []int16) {
		g.samples = samples
	}
	g.radio.SMeterCallback = func(smeter int) {
		g.smeterValue = smeter
	}

	shutdown := func() {
		g.quit = true
		g.radio.Stop()
		os.Exit(0)
	}

	defer shutdown()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		s := <-sigc
		log.Println(s)
		shutdown()
	}()

	go func() {
		if *reset {
			log.Println("Resetting board")
			g.radio.Reset()
			time.Sleep(1 * time.Second)
		}

		// Wait for HELLO message
		for i := 0; i < 2; i++ {
			if g.radio.Hello() {
				break
			}

			if i > 0 {
				log.Println("Reset board")
				g.radio.Reset()
			}

			for j := 0; j < 10 && !g.radio.Hello(); j++ {
				log.Println("Waiting for HELLO message...")
				time.Sleep(1 * time.Second)
			}
		}

		if !g.radio.Hello() {
			log.Println("No HELLO message received")
			return
		}

		if err := g.radio.SendStop(); err != nil {
			log.Fatalf("Send STOP: %v", err)
			return
		}

		if *freq < kv4pht.VHF_MIN_FREQ {
			*freq = kv4pht.VHF_MIN_FREQ
		} else if *freq > kv4pht.VHF_MAX_FREQ && *freq < kv4pht.UHF_MIN_FREQ {
			*freq = kv4pht.VHF_MAX_FREQ
		} else if *freq > kv4pht.UHF_MAX_FREQ {
			*freq = kv4pht.UHF_MAX_FREQ
		}

		mode := kv4pht.MODE_VHF
		if *band == "uhf" || *freq >= kv4pht.UHF_MIN_FREQ {
			mode = kv4pht.MODE_UHF
		}
		if err := g.radio.SendConfig(mode); err != nil {
			log.Fatalf("Send CONFIG: %v", err)
			return
		}

		// Wait for VERSION message
		for i := 0; i < 10; i++ {
			v, _, _ := g.radio.Version()
			if v != 0 {
				break
			}

			log.Println("Waiting for VERSION message...")
			time.Sleep(1 * time.Second)
		}

		if v, _, _ := g.radio.Version(); v == 0 {
			log.Println("No VERSION message received")
			return
		}

		if err := g.radio.SendFilters(*pre, *high, *low); err != nil {
			log.Fatalf("Send FILTERS: %v", err)
			return
		}

		g.radio.SetVolume(0.5)

		g.bw = kv4pht.DRA818_25K
		if *bw != "wide" {
			g.bw = kv4pht.DRA818_12K5
		}

		if *squelch < 0 {
			*squelch = 0
		} else if *squelch > 100 {
			*squelch = 100
		}

		g.squelch = 255 * *squelch / 100 // squelch is actually 0-255
		g.freq = float64(*freq)
		g.numberInput.SetValue(int(g.freq * 1000000))
		g.numberInput.ValueCallback = func(value int) {
			g.freq = float64(value) / 1000000

			if err := g.radio.SendGroup(g.bw, g.freq, g.freq, g.squelch); err != nil {
				log.Printf("Send GROUP: %v", err)
			}
		}

		if err := g.radio.SendGroup(g.bw, g.freq, g.freq, g.squelch); err != nil {
			log.Fatalf("Send GROUP: %v", err)
			return
		}
	}()

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("KV4P-HT Radio")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
