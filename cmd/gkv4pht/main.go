package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	kv4pht "github.com/raff/kv4p-go"
)

const fontSize = 28

var (
	whiteImage = ebiten.NewImage(3, 3)

	// whiteSubImage is an internal sub image of whiteImage.
	// Use whiteSubImage at DrawTriangles instead of whiteImage in order to avoid bleeding edges.
	whiteSubImage = whiteImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)

	largeFont text.Face
	smallFont text.Face
)

func init() {
	whiteImage.Fill(color.White)

	ff, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		log.Fatal("error loading font", err)
	}

	largeFont = &text.GoTextFace{Source: ff, Size: 28}
	smallFont = &text.GoTextFace{Source: ff, Size: 18}
}

const (
	screenWidth  = 400
	screenHeight = 480
)

type Game struct {
	samples []int16

	numberInput *NumberInput
	waveform    *Waveform
	smeter      *SMeter
	band        *ToggleButton
	pre         *ToggleButton
	high        *ToggleButton
	low         *ToggleButton
	scan        *ToggleButton
	bandwidth   *ToggleButton
	quit        bool

	radio   *kv4pht.CommandProcessor
	mode    int
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

	dw, dh := text.Measure("0 ", largeFont, 0)
	m := largeFont.Metrics()
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
	return float32(n.bounds.Dx()), float32(n.bounds.Dy())
}

func (n *NumberInput) SetLimits(minValue, maxValue int) {
	n.minValue = minValue
	n.maxValue = maxValue
	if n.value < minValue {
		n.value = minValue
	}
	if n.value > maxValue {
		n.value = maxValue
	}
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
		text.Draw(screen, string(ch), largeFont, op)
	}

	// Draw cursor if editing
	if n.editing && n.cursor < n.maxDigits {
		x := int(n.x) + n.cursor*int(n.dw)
		vector.DrawFilledRect(screen, float32(x), n.y+n.dh-4, n.dw, 2, color.RGBA{0xff, 0xff, 0xff, 0xff}, false) // anti-aliased
	}
}

type ToggleButton struct {
	x, y, w, h float32
	label      string
	value      bool
	onClick    func(bool)
}

func NewToggleButton(x, y, w, h float32, label string, value bool, onClick func(bool)) *ToggleButton {
	return &ToggleButton{
		x:       x,
		y:       y,
		w:       w,
		h:       h,
		label:   label,
		value:   value,
		onClick: onClick,
	}
}

func (b *ToggleButton) SetValue(value bool) {
	b.value = value
	if b.onClick != nil {
		b.onClick(b.value)
	}
}

func (b *ToggleButton) Update() {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if float32(x) >= b.x && float32(x) <= b.x+b.w &&
			float32(y) >= b.y && float32(y) <= b.y+b.h {
			b.SetValue(!b.value)
		}
	}
}

func (b *ToggleButton) Draw(screen *ebiten.Image) {
	// Draw background
	colors := map[bool]color.RGBA{
		false: {0x99, 0x99, 0x99, 0xff},
		true:  {0x33, 0x33, 0x33, 0xff},
	}

	vector.DrawFilledRect(screen, b.x, b.y, b.w/2, b.h, colors[b.value], false)
	vector.DrawFilledRect(screen, b.x+b.w/2, b.y, b.w/2, b.h, colors[!b.value], false)

	// Draw label
	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(b.x+4), float64(b.y+8))
	op.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, b.label, smallFont, op)
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
	g.scan.Draw(screen)
	g.bandwidth.Draw(screen)
	g.smeter.Draw(screen)
	g.band.Draw(screen)
	g.pre.Draw(screen)
	g.high.Draw(screen)
	g.low.Draw(screen)
}

func (g *Game) Update() error {
	if g.quit {
		return ebiten.Termination
	}

	g.numberInput.Update()

	g.waveform.Update(g.samples[:])
	g.scan.Update()
	g.bandwidth.Update()
	g.smeter.Update(g.smeterValue)
	g.band.Update()
	g.pre.Update()
	g.high.Update()
	g.low.Update()
	return nil
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) Scan(freq float64) {
	var min, max, step float64

	if g.mode == kv4pht.MODE_VHF {
		min, max = freq, float64(kv4pht.VHF_MAX_FREQ)
	} else {
		min, max = freq, float64(kv4pht.UHF_MAX_FREQ)
	}

	if g.bw == kv4pht.DRA818_25K {
		step = 0.025
	} else {
		step = 0.0125
	}

	if err := g.radio.SendFilters(false, false, false); err != nil {
		log.Printf("Send FILTERS: %v", err)
		return
	}

	if err := g.radio.SendGroup(g.bw, min, min, 4); err != nil { // it seems that scan needs to be sent with a squelch > 0
		log.Printf("Send GROUP: %v", err)
		return
	}

freq_loop:
	for f := min; f <= max; f += step {
		g.numberInput.SetValue(int(f * 1000000))

		if err := g.radio.SendScan(f); err != nil {
			log.Printf("Send SCAN: %v", err)
			return
		}

		for i := 0; i < 10; i++ {
			if g.radio.Scanned() != kv4pht.SCAN_WAITING {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		scanned := g.radio.Scanned()
		switch {
		case scanned == kv4pht.SCAN_WAITING:
			log.Fatal("No SCAN response received")

		case scanned == kv4pht.SCAN_NOT_FOUND:
			continue

		case scanned&kv4pht.SCAN_FOUND != 0:
			log.Printf("SCAN found %v", f)
			g.freq = f
			break freq_loop

		default:
			log.Printf("SCAN unknown response %v", g.radio.Scanned())
		}
	}

	g.numberInput.SetValue(int(g.freq * 1000000))

	if err := g.radio.SendFilters(g.pre.value, g.high.value, g.low.value); err != nil {
		log.Printf("Send FILTERS: %v", err)
		return
	}
	if err := g.radio.SendGroup(g.bw, g.freq, g.freq, g.squelch); err != nil {
		log.Printf("Send GROUP: %v", err)
	}
}

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	flag.BoolVar(&kv4pht.Debug, "debug", kv4pht.Debug, "Enable debug output")

	band := flag.String("band", "vhf", "Band (vhf, uhf)")
	bw := flag.String("bw", "wide", "Bandwidth (wide=25k, narrow=12.5k)")
	freq := flag.Float64("freq", 162.4, "Frequency in MHz") // NOAA Weather Radio
	squelch := flag.Int("squelch", 0, "Squelch level (0-8)")
	pre := flag.Bool("pre", true, "pre-emphasis filter")
	high := flag.Bool("high", true, "high-pass filter")
	low := flag.Bool("low", true, "low-pass filter")
	reset := flag.Bool("reset", false, "reset board")
	flag.Parse()

	if *freq < kv4pht.VHF_MIN_FREQ {
		*freq = kv4pht.VHF_MIN_FREQ
	} else if *freq > kv4pht.VHF_MAX_FREQ && *freq < kv4pht.UHF_MIN_FREQ {
		*freq = kv4pht.VHF_MAX_FREQ
	} else if *freq > kv4pht.UHF_MAX_FREQ {
		*freq = kv4pht.UHF_MAX_FREQ
	}

	g := &Game{}

	left := float32(20)
	top := float32(20)

	minfreq := int(kv4pht.VHF_MIN_FREQ * 1000000)
	maxfreq := int(kv4pht.VHF_MAX_FREQ * 1000000)
	if *band == "uhf" || *freq >= kv4pht.UHF_MIN_FREQ {
		minfreq = int(kv4pht.UHF_MIN_FREQ * 1000000)
		maxfreq = int(kv4pht.UHF_MAX_FREQ * 1000000)
	}

	radio, err := kv4pht.Start(*dev)
	if err != nil {
		log.Fatalf("Start: %v", err)
	}

	g.numberInput = NewNumberInput(minfreq, maxfreq, left, top)
	w, h := g.numberInput.Size()

	g.band = NewToggleButton(left+w+20, top, w/2-10, h, "VHF   UHF", true, func(value bool) {

		if value {
			g.mode = kv4pht.MODE_VHF
			minfreq := int(kv4pht.VHF_MIN_FREQ * 1000000)
			maxfreq := int(kv4pht.VHF_MAX_FREQ * 1000000)
			g.numberInput.SetLimits(minfreq, maxfreq)
		} else {
			g.mode = kv4pht.MODE_UHF
			minfreq := int(kv4pht.UHF_MIN_FREQ * 1000000)
			maxfreq := int(kv4pht.UHF_MAX_FREQ * 1000000)
			g.numberInput.SetLimits(minfreq, maxfreq)
		}

		if err := g.radio.SendConfig(g.mode); err != nil {
			log.Printf("Send CONFIG: %v", err)
		}
	})

	top += h + 10
	g.smeter = NewSMeter(left, top, w, h/2)

	top += h/2 + 10
	g.waveform = NewWaveform(left, top, w, h)

	g.bandwidth = NewToggleButton(left+w+20, top, w/2-10, h, "Wide Narr.", g.bw == kv4pht.DRA818_25K, func(value bool) {
		if value {
			g.bw = kv4pht.DRA818_25K
		} else {
			g.bw = kv4pht.DRA818_12K5
		}

		if err := g.radio.SendGroup(g.bw, g.freq, g.freq, g.squelch); err != nil {
			log.Printf("Send GROUP: %v", err)
		}
	})

	top += h + 10
	g.pre = NewToggleButton(left, top, w, h, "Pre-emph.", *pre, func(value bool) {
		if err := g.radio.SendFilters(g.pre.value, g.high.value, g.low.value); err != nil {
			log.Printf("Send FILTERS: %v", err)
		}
	})

	g.scan = NewToggleButton(left+w+20, top, w/2-10, h, "Scan", false, func(value bool) {
		if value {
			go func() {
				g.Scan(g.freq)
				g.scan.SetValue(false)
			}()
		}
	})

	top += h + 10
	g.high = NewToggleButton(left, top, w, h, "High-pass", *high, func(value bool) {
		if err := g.radio.SendFilters(g.pre.value, g.high.value, g.low.value); err != nil {
			log.Printf("Send FILTERS: %v", err)
		}
	})

	top += h + 10
	g.low = NewToggleButton(left, top, w, h, "Low-pass", *high, func(value bool) {
		if err := g.radio.SendFilters(g.pre.value, g.high.value, g.low.value); err != nil {
			log.Printf("Send FILTERS: %v", err)
		}
	})

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

		g.mode = kv4pht.MODE_VHF
		if *band == "uhf" || *freq >= kv4pht.UHF_MIN_FREQ {
			g.mode = kv4pht.MODE_UHF
		}
		if err := g.radio.SendConfig(g.mode); err != nil {
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
		} else if *squelch > 8 {
			*squelch = 8
		}

		g.squelch = *squelch
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
