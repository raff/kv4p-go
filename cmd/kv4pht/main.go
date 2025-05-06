package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/raff/kv4p-go"
)

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	reset := flag.Bool("reset", false, "Reset board")
	wait := flag.Duration("wait", 60*time.Second, "Receive time before exiting")
	flag.BoolVar(&kv4pht.Debug, "debug", kv4pht.Debug, "Enable debug output")

	band := flag.String("band", "vhf", "Band (vhf, uhf)")
	bw := flag.String("bw", "wide", "Bandwidth (wide=25k, narrow=12.5k)")
	freq := flag.Float64("freq", 162.4, "Frequency in MHz") // NOAA Weather Radio
	squelch := flag.Int("squelch", 0, "Squelch level (0-100)")
	pre := flag.Bool("pre", false, "pre-emphasis filter")
	high := flag.Bool("high", true, "high-pass filter")
	low := flag.Bool("low", true, "low-pass filter")
	scan := flag.Bool("scan", false, "Scan selected band")

	volume := flag.Int("volume", 100, "Volume (0-100)")
	flag.Parse()

	p, err := kv4pht.Start(*dev)
	if err != nil {
		log.Fatalf("Start: %v", err)
	}

	shutdown := func() {
		p.Stop()
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

	if *reset {
		log.Println("Resetting board")
		p.Reset()
		time.Sleep(1 * time.Second)
	}

	/*
		if *dtr {
			log.Println("Set DTR")
			if err := p.port.SetDTR(true); err != nil {
				log.Fatalf("SetDTR: %v", err)
			}
		}
		if *rts {
			log.Println("Set RTS")
			if err := p.port.SetRTS(true); err != nil {
				log.Fatalf("SetRTS: %v", err)
			}
		}

		if *sbreak {
			log.Println("Sending BREAK")
			p.port.Break(10 * time.Millisecond)
		}
	*/

	// Wait for HELLO message
	for i := 0; i < 2; i++ {
		if p.Hello() {
			break
		}

		if i > 0 {
			log.Println("Reset board")
			p.Reset()
		}

		for j := 0; j < 10 && !p.Hello(); j++ {
			log.Println("Waiting for HELLO message...")
			time.Sleep(1 * time.Second)
		}
	}

	if !p.Hello() {
		log.Println("No HELLO message received")
		return
	}

	if err := p.SendStop(); err != nil {
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
	if err := p.SendConfig(mode); err != nil {
		log.Fatalf("Send CONFIG: %v", err)
		return
	}

	// Wait for VERSION message
	for i := 0; i < 10; i++ {
		v, _, _ := p.Version()
		if v != 0 {
			break
		}

		log.Println("Waiting for VERSION message...")
		time.Sleep(1 * time.Second)
	}

	if v, _, _ := p.Version(); v == 0 {
		log.Println("No VERSION message received")
		return
	}

	if err := p.SendFilters(*pre, *high, *low); err != nil {
		log.Fatalf("Send FILTERS: %v", err)
		return
	}

	if *volume < 0 {
		*volume = 0
	} else if *volume > 100 {
		*volume = 100
	}
	p.SetVolume(float64(*volume) / 100)

	rbw := kv4pht.DRA818_25K
	if *bw != "wide" {
		rbw = kv4pht.DRA818_12K5
	}

	if *squelch < 0 {
		*squelch = 0
	} else if *squelch > 100 {
		*squelch = 100
	}

	*squelch = 255 * *squelch / 100 // squelch is actually 0-255

	if *scan {
		var min, max, step float64

		if mode == kv4pht.MODE_VHF {
			min, max = float64(*freq), float64(kv4pht.VHF_MAX_FREQ)
		} else {
			min, max = float64(*freq), float64(kv4pht.UHF_MAX_FREQ)
		}

		if rbw == kv4pht.DRA818_25K {
			step = 0.025
		} else {
			step = 0.0125
		}

		fmt.Println("SCANNING...")
	freq_loop:
		for f := min; f <= max; f += step {
			log.Printf("FREQ: %3.3f", f)
			if err := p.SendGroup(rbw, f, f, *squelch); err != nil {
				log.Fatalf("Send GROUP: %v", err)
				return
			}

			_, start := p.SMeter()

			for {
				smeter, scount := p.SMeter()

				if kv4pht.Debug {
					log.Printf("...%v (%v)", scount-start, smeter)
				}

				if smeter > 3 || scount-start > 20 {
					break freq_loop
				}

				time.Sleep(100 * time.Millisecond)
			}
		}

		fmt.Println("SCAN Done")
	} else {
		log.Printf("FREQ: %3.3f", *freq)
		if err := p.SendGroup(rbw, *freq, *freq, *squelch); err != nil {
			log.Fatalf("Send GROUP: %v", err)
			return
		}
	}

	fmt.Println("Press Ctrl+C to exit")
	time.Sleep(*wait)
}
