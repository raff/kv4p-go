package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"waveform/widgets"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/guigui"
	"github.com/hajimehoshi/guigui/basicwidget"

	kv4pht "github.com/raff/kv4p-go"
)

const (
	screenWidth  = 330
	screenHeight = 480
)

const (
	BAND_VHF = 0
	BAND_UHF = 1

	BW_25   = 0
	BW_12_5 = 1
)

type Root struct {
	guigui.DefaultWidget

	width int

	background basicwidget.Background
	form       basicwidget.Form

	freqText     basicwidget.Text
	bandText     basicwidget.Text
	bwText       basicwidget.Text
	waveformText basicwidget.Text
	squelchText  basicwidget.Text
	smeterText   basicwidget.Text

	freqInput widgets.NumberInput
	bands     basicwidget.SegmentedControl[int]
	bws       basicwidget.SegmentedControl[int]
	waveform  widgets.Waveform
	ssquelch  basicwidget.Slider
	smeter    widgets.Bar

	quit bool
	tick int

	samples []int16

	radio   *kv4pht.CommandProcessor
	mode    int
	bw      int
	squelch int
	freq    float64
}

func (r *Root) SetMode(mode int) {
	r.mode = mode

	if r.mode == kv4pht.MODE_VHF {
		minfreq := int(kv4pht.VHF_MIN_FREQ * 1000000)
		maxfreq := int(kv4pht.VHF_MAX_FREQ * 1000000)
		r.freqInput.SetLimits(minfreq, maxfreq)
	} else {
		minfreq := int(kv4pht.UHF_MIN_FREQ * 1000000)
		maxfreq := int(kv4pht.UHF_MAX_FREQ * 1000000)
		r.freqInput.SetLimits(minfreq, maxfreq)
	}
}

func (r *Root) Build(context *guigui.Context, appender *guigui.ChildWidgetAppender) error {
	// log.Println("Root.Build")

	bounds := context.Bounds(r)

	appender.AppendChildWidgetWithBounds(&r.background, bounds)
	context.SetColorMode(guigui.ColorModeDark)

	r.bands.SetItems([]basicwidget.SegmentedControlItem[int]{
		{Text: "VHF", ID: kv4pht.MODE_VHF},
		{Text: "UHF", ID: kv4pht.MODE_UHF},
	})
	r.bands.SetDirection(basicwidget.SegmentedControlDirectionHorizontal)
	r.bands.SetOnItemSelected(func(i int) {
		if item, ok := r.bands.ItemByIndex(i); ok && item.ID != r.mode {
			r.SetMode(item.ID)

			if err := r.radio.SendConfig(r.mode); err != nil {
				log.Printf("Send CONFIG: %v", err)
			}

		}
	})
	r.bands.SelectItemByID(r.mode)

	r.bws.SetItems([]basicwidget.SegmentedControlItem[int]{
		{Text: "25 kHz", ID: kv4pht.DRA818_25K},
		{Text: "12.5 kHz", ID: kv4pht.DRA818_12K5},
	})
	r.bws.SetDirection(basicwidget.SegmentedControlDirectionHorizontal)
	r.bws.SetOnItemSelected(func(i int) {
		if item, ok := r.bws.ItemByIndex(i); ok && item.ID != r.bw {
			r.bw = item.ID

			if r.bw == kv4pht.DRA818_25K {
				r.freqInput.SetStep(25000)
			} else {
				r.freqInput.SetStep(12500)
			}

			if err := r.radio.SendGroup(r.bw, r.freq, r.freq, r.squelch); err != nil {
				log.Printf("Send GROUP: %v", err)
			}
		}
	})
	r.bws.SelectItemByID(r.bw)

	r.freqText.SetValue("Frequency")
	r.bandText.SetValue("Band")
	r.bwText.SetValue("Bandwidth")
	r.squelchText.SetValue("Squelch")
	r.smeterText.SetValue("S-Meter")
	r.waveformText.SetValue("Audio")

	u := basicwidget.UnitSize(context)
	swidth := context.Bounds(r).Dx() / 3 * 2
	context.SetSize(&r.freqInput, image.Pt(swidth, 3*u))
	context.SetSize(&r.bands, image.Pt(swidth, guigui.DefaultSize))
	context.SetSize(&r.bws, image.Pt(swidth, guigui.DefaultSize))
	context.SetSize(&r.ssquelch, image.Pt(swidth, guigui.DefaultSize))
	context.SetSize(&r.smeter, image.Pt(swidth, 1*u))
	context.SetSize(&r.waveform, image.Pt(swidth, 6*u))

	r.form.SetItems([]basicwidget.FormItem{
		{
			PrimaryWidget:   &r.freqText,
			SecondaryWidget: &r.freqInput,
		},
		{
			PrimaryWidget:   &r.bandText,
			SecondaryWidget: &r.bands,
		},
		{
			PrimaryWidget:   &r.bwText,
			SecondaryWidget: &r.bws,
		},
		{
			PrimaryWidget:   &r.squelchText,
			SecondaryWidget: &r.ssquelch,
		},
		{
			PrimaryWidget:   &r.smeterText,
			SecondaryWidget: &r.smeter,
		},
		{
			PrimaryWidget:   &r.waveformText,
			SecondaryWidget: &r.waveform,
		},
	})

	appender.AppendChildWidgetWithBounds(&r.form, context.Bounds(r).Inset(10))
	return nil
}

func (r *Root) Tick(context *guigui.Context) error {
	//log.Println("Root.Tick")

	if r.quit {
		return ebiten.Termination
	}

	r.waveform.Update(context, r.samples)
	return nil
}

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	flag.BoolVar(&kv4pht.Debug, "debug", kv4pht.Debug, "Enable debug output")

	band := flag.String("band", "vhf", "Band (vhf, uhf)")
	bw := flag.String("bw", "wide", "Bandwidth (wide=25k, narrow=12.5k)")
	freq := flag.Float64("freq", 162.55, "Frequency in MHz") // NOAA Weather Radio
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

	r := &Root{}

	if *bw == "wide" {
		r.bw = kv4pht.DRA818_25K
		r.freqInput.SetStep(25000)
	} else if *bw == "narrow" {
		r.bw = kv4pht.DRA818_12K5
		r.freqInput.SetStep(12500)
	} else {
		log.Fatalf("Invalid bandwidth: %s", *bw)
	}

	if *squelch < 0 {
		*squelch = 0
	} else if *squelch > 8 {
		*squelch = 8
	}

	if *band == "vhf" {
		r.SetMode(kv4pht.MODE_VHF)
	} else if *band == "uhf" || *freq >= kv4pht.UHF_MIN_FREQ {
		r.SetMode(kv4pht.MODE_UHF)
	}

	r.squelch = *squelch
	r.freq = *freq

	r.freqInput.SetValue(int(r.freq * 1000000))
	r.freqInput.SetSeparator(',')
	r.freqInput.SetOnValueChanged(func(v int) {
		r.freq = float64(v) / 1000000

		if err := r.radio.SendGroup(r.bw, r.freq, r.freq, r.squelch); err != nil {
			log.Printf("Send GROUP: %v", err)
		}
	})

	r.ssquelch.SetMinimumValueInt64(0)
	r.ssquelch.SetMaximumValueInt64(8)
	r.ssquelch.SetOnValueChangedInt64(func(v int64) {
		if v != int64(r.squelch) {
			r.squelch = int(v)

			if err := r.radio.SendGroup(r.bw, r.freq, r.freq, r.squelch); err != nil {
				log.Printf("Send GROUP: %v", err)
			}
		}
	})

	r.bws.SelectItemByID(r.bw)
	r.smeter.SetLimits(0, 9)

	radio, err := kv4pht.Start(*dev)
	if err != nil {
		log.Fatalf("Start: %v", err)
	}

	r.radio = radio

	r.radio.AudioCallback = func(samples []int16) {
		r.samples = samples
	}
	r.radio.SMeterCallback = func(smeter int) {
		r.smeter.SetValue(smeter)
	}

	shutdown := func() {
		r.quit = true
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
			r.radio.Reset()
			time.Sleep(1 * time.Second)
		}

		// Wait for HELLO message
		for i := 0; i < 3; i++ {
			if r.radio.Hello() {
				break
			}

			if i > 0 {
				log.Println("Reset board")
				r.radio.Reset()
			}

			for j := 0; j < 5 && !r.radio.Hello(); j++ {
				log.Println("Waiting for HELLO message...")
				time.Sleep(1 * time.Second)
			}
		}

		if !r.radio.Hello() {
			log.Println("No HELLO message received")
			return
		}

		if err := r.radio.SendStop(); err != nil {
			log.Fatalf("Send STOP: %v", err)
			return
		}

		if err := r.radio.SendConfig(r.mode); err != nil {
			log.Fatalf("Send CONFIG: %v", err)
			return
		}

		// Wait for VERSION message
		for i := 0; i < 10; i++ {
			v, _, _ := r.radio.Version()
			if v != 0 {
				break
			}

			log.Println("Waiting for VERSION message...")
			time.Sleep(1 * time.Second)
		}

		if v, s, hw := r.radio.Version(); v == 0 {
			log.Println("No VERSION message received")
			return
		} else {
			ebiten.SetWindowTitle(fmt.Sprintf("KV4P-HT v%v s=%v hw=%v", v, s, hw))
		}

		if err := r.radio.SendFilters(*pre, *high, *low); err != nil {
			log.Fatalf("Send FILTERS: %v", err)
			return
		}

		r.radio.SetVolume(0.5)

		if err := r.radio.SendGroup(r.bw, r.freq, r.freq, r.squelch); err != nil {
			log.Fatalf("Send GROUP: %v", err)
			return
		}
	}()

	op := &guigui.RunOptions{
		Title:         "KV4P-HT",
		WindowSize:    image.Pt(screenWidth, screenHeight),
		WindowMinSize: image.Pt(screenWidth, screenHeight),
		RunGameOptions: &ebiten.RunGameOptions{
			ApplePressAndHoldEnabled: true,
		},
	}
	if err := guigui.Run(r, op); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
