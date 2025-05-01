package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ebitengine/oto/v3"
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
	"gopkg.in/hraban/opus.v2"
)

var (
	// Must match the ESP32 device we support.
	// Idx 0 matches https://www.amazon.com/gp/product/B08D5ZD528
	esp32_vendor_ids  = []string{"10C4", "1A86"}
	esp32_product_ids = []string{"EA60", "7523"}

	cmd_prefix = []byte{0xDE, 0xAD, 0xBE, 0xEF}
	debug      = false
)

const (
	// Command codes
	CMD_PTT_DOWN      = 0x01
	CMD_PTT_UP        = 0x02
	CMD_GROUP         = 0x03
	CMD_FILTERS       = 0x04
	CMD_STOP          = 0x05
	CMD_CONFIG        = 0x06
	CMD_TX_AUDIO      = 0x07
	CMD_WINDOW_UPDATE = 0x08

	// Response codes
	RES_SMETER_REPORT = 0x53
	RES_PHYS_PTT_DOWN = 0x44
	RES_PHYS_PTT_UP   = 0x55
	RES_DEBUG_INFO    = 0x01
	RES_DEBUG_ERROR   = 0x02
	RES_DEBUG_WARNING = 0x03
	RES_DEBUG_DEBUG   = 0x04
	RES_DEBUG_TRACE   = 0x05
	RES_HELLO         = 0x06
	RES_RX_AUDIO      = 0x07
	RES_VERSION       = 0x08
	RES_WINDOW_UPDATE = 0x09

	MODE_VHF = 0x04
	MODE_UHF = 0x05

	DRA818_25K  = 0x01
	DRA818_12K5 = 0x00

	VHF_MIN_FREQ = 134.0 // SA818U lower limit, in MHz
	VHF_MAX_FREQ = 174.0 // SA818U upper limit, in MHz
	UHF_MIN_FREQ = 400.0 // SA818U lower limit, in MHz
	UHF_MAX_FREQ = 480.0 // SA818U upper limit, in MHz (DRA818U can only go to 470MHz)

	AUDIO_SAMPLING_RATE = 48000 // 48kHz
	OPUS_FRAME_SIZE     = 1920  // 40ms at 48kHz
)

type Group struct {
	bw       byte // Bandwidth (25kHz, 12.5kHz)
	freq_tx  float32
	freq_rx  float32
	ctxss_tx byte
	squelch  byte
	ctxss_rx byte
}

type CommandProcessor struct {
	state  int
	cmd    byte
	plen   int
	params []byte

	version     uint16
	radioStatus byte
	hwver       byte
	windowSize  int

	hello bool
	quit  bool

	audioDecoder *opus.Decoder
	audioBuffer  []int16
	player       *oto.Player
}

func (p *CommandProcessor) processBytes(buf []byte) {
	for _, b := range buf {
		switch {
		case p.state < len(cmd_prefix):
			if b != cmd_prefix[p.state] {
				p.params = append(p.params, b)
			} else {
				if len(p.params) > 0 {
					log.Print("Skipped bytes:\n", hex.Dump(p.params))
					p.params = nil
				}
				p.state++
			}

		case p.state == len(cmd_prefix):
			p.cmd = b
			p.state++

		case p.state == len(cmd_prefix)+1:
			//log.Println("plen-1", b)
			p.plen = int(b) & 0xFF
			p.state++

		case p.state == len(cmd_prefix)+2:
			//log.Println("plen-2", b)
			p.plen |= (int(b) & 0xFF) << 8
			p.state++

			if p.plen == 0 {
				p.params = nil
				p.processCommand()
				break
			}

			p.params = make([]byte, 0, p.plen)

		default:
			l := len(p.params)
			if l < p.plen {
				p.params = append(p.params, b)
			}

			if l == p.plen-1 {
				p.processCommand()
				break
			}
		}
	}
}

func (p *CommandProcessor) processCommand() {
	if debug {
		log.Println("processCommand", p.cmd, p.plen) //, p.params)
	}

	switch p.cmd {
	case RES_DEBUG_INFO:
		log.Printf("INFO: %s", string(p.params))
	case RES_DEBUG_ERROR:
		log.Printf("ERROR: %s", string(p.params))
	case RES_DEBUG_WARNING:
		log.Printf("WARNING: %s", string(p.params))
	case RES_DEBUG_DEBUG:
		log.Printf("DEBUG: %s", string(p.params))
	case RES_DEBUG_TRACE:
		log.Printf("TRACE %s", string(p.params))
	case RES_PHYS_PTT_DOWN:
		log.Println("PTT BUTTON DOWN")
	case RES_PHYS_PTT_UP:
		log.Println("PTT BUTTON UP")
	case RES_HELLO:
		log.Printf("HELLO: %s\n", p.params)
		p.hello = true
	case RES_VERSION:
		if p.plen != 8 {
			log.Printf("Invalid version length: %d (%02x)\n", p.plen, p.params)
			break
		}
		p.version = binary.LittleEndian.Uint16(p.params[0:2])
		p.radioStatus = p.params[2]
		p.hwver = p.params[3]
		p.windowSize = int(binary.LittleEndian.Uint32(p.params[4:8]))
		log.Printf("Version: %d, rstatus: %c, hwver: %02x, windowSize: %d\n", p.version, p.radioStatus, p.hwver, p.windowSize)
	case RES_WINDOW_UPDATE:
		if p.plen != 4 {
			log.Printf("Invalid window update length: %d (%02x)\n", p.plen, p.params)
			break
		}
		wsize := binary.LittleEndian.Uint32(p.params[0:4])
		p.windowSize += int(wsize)
		log.Printf("Window update: %d\n", p.windowSize)
	case RES_SMETER_REPORT:
		if p.plen != 1 {
			log.Printf("Invalid S-Meter length: %d (%02x)\n", p.plen, p.params)
			break
		}
		smeter := int(p.params[0]) & 0xFF
		log.Printf("S-Meter: %d\n", smeterValue(smeter))
	case RES_RX_AUDIO:
		if debug {
			log.Printf("RX AUDIO (%v bytes):", p.plen)
			h := p.params[0]
			cn := (h & 0xF8) >> 3
			s := (h & 0x04) >> 2
			fc := (h & 0x03)
			log.Printf("Header: %02x, conf: %d, silk: %d, frame-code: %d\n", h, cn, s, fc)
		}

		var out [OPUS_FRAME_SIZE]int16
		if n, err := p.audioDecoder.Decode(p.params, out[:]); err != nil {
			log.Printf("Decode: %v\n", err)
			fmt.Println(toByteArray(p.params))
		} else {
			if debug {
				log.Printf("Decoded %d samples\n", n)
			}

			p.audioBuffer = append(p.audioBuffer, out[:n]...)
		}

	default:
		fmt.Printf("Unknown command %02x: %02x\n", p.cmd, p.params)
	}

	p.state = 0
	p.cmd = 0
	p.plen = 0
	p.params = nil
}

func (p *CommandProcessor) newCommand(cmd byte, params []byte) []byte {
	l := len(params)
	buffer := make([]byte, 4+1+2+l)
	copy(buffer[:4], cmd_prefix)
	buffer[4] = cmd
	binary.LittleEndian.PutUint16(buffer[5:], uint16(l))
	if l > 0 {
		copy(buffer[7:], params)
	}

	if debug {
		log.Printf("newCommand %02x: %02x\n", cmd, buffer)
	}
	l = len(buffer)
	if l > p.windowSize {
		log.Printf("Window size exceeded: %d > %d\n", l, p.windowSize)
		time.Sleep(1 * time.Second)
	}
	p.windowSize -= l
	return buffer
}

// implement io.Reader interface for oto.Player
func (p *CommandProcessor) Read(buf []byte) (int, error) {
	if len(p.audioBuffer) == 0 {
		for i := 0; i < len(buf); i++ {
			buf[i] = 0
		}

		return len(buf), nil
	}

	la := len(p.audioBuffer)
	lb := la * 2

	if lb > len(buf) {
		lb = len(buf)
		la = lb / 2
	}

	j := 0
	for i := 0; i < lb; i += 2 {
		buf[i+0] = byte(p.audioBuffer[j])
		buf[i+1] = byte(p.audioBuffer[j] >> 8)
		j++
	}

	p.audioBuffer = p.audioBuffer[la:]
	return lb, nil
}

func smeterValue(s255 int) int {
	result := 9.73*math.Log(0.0297*float64(s255)) - 1.88
	return max(1, min(9, int(math.Round(result))))
}

func toByteArray(b []byte) string {
	var result strings.Builder
	result.WriteString("  {\n    ")
	for _, c := range b {
		if c >= 0x20 && c <= 0x7E && c != '\'' && c != '\\' {
			result.WriteString(fmt.Sprintf("'%c', ", c))
		} else {
			result.WriteString(fmt.Sprintf("0x%02x, ", c))
		}
	}
	result.WriteString("\n  },")
	return result.String()
}

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	baud := flag.Int("baud", 115200, "Baud rate")
	sbreak := flag.Bool("break", false, "Send BREAK signal")
	dtr := flag.Bool("dtr", false, "Set DTR")
	rts := flag.Bool("rts", false, "Set RTS")
	wait := flag.Duration("wait", 60*time.Second, "Receive time before exiting")
	flag.BoolVar(&debug, "debug", debug, "Enable debug output")

	band := flag.String("band", "vhf", "Band (vhf, uhf)")
	bw := flag.String("bw", "wide", "Bandwidth (wide=25k, narrow=12.5k)")
	freq := flag.Float64("freq", 162.4, "Frequency in MHz") // NOAA Weather Radio
	squelch := flag.Int("squelch", 0, "Squelch level (0-255)")

	volume := flag.Float64("volume", 1.0, "Volume (0.0-1.0)")
	flag.Parse()

	if *dev == "" {
		ports, err := enumerator.GetDetailedPortsList()
		if err != nil {
			log.Fatalf("GetPorts: %v", err)
		}
		if len(ports) == 0 {
			fmt.Println("No serial ports found!")
			return
		}

		for _, port := range ports {
			if port.IsUSB {
				for i, id := range esp32_vendor_ids {
					if port.VID == id && port.PID == esp32_product_ids[i] {
						*dev = port.Name
						fmt.Printf("Found ESP32 device: %s\n", port.Name)
						break
					}
				}
			}
		}

		if *dev == "" {
			fmt.Println("No ESP32 device found!")
			return
		}
	}

	smode := &serial.Mode{
		BaudRate: *baud,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	port, err := serial.Open(*dev, smode)
	if err != nil {
		log.Fatalf("Open %v: %v", *dev, err)
	}

	p := &CommandProcessor{windowSize: 1024}
	p.audioDecoder, err = opus.NewDecoder(AUDIO_SAMPLING_RATE, 1)
	if err != nil {
		log.Fatalf("NewDecoder: %v", err)
	}

	op := &oto.NewContextOptions{
		SampleRate:   AUDIO_SAMPLING_RATE,
		ChannelCount: 1,
		Format:       oto.FormatSignedInt16LE,
	}
	c, ready, err := oto.NewContext(op)
	if err != nil {
		log.Fatalf("Audio NewContext: %v", err)
	}
	<-ready

	p.player = c.NewPlayer(p)

	shutdown := func() {
		p.quit = true

		log.Println("Sending STOP command")
		if _, err := port.Write(p.newCommand(CMD_STOP, nil)); err != nil {
			log.Fatalf("Send STOP: %v", err)
			return
		}
		port.Drain()

		time.Sleep(1 * time.Second)

		p.player.Close()
		port.Close()
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

	// Read from the serial port
	go func() {
		buf := make([]byte, 1024)

		for {
			if p.quit {
				break
			}

			n, err := port.Read(buf)
			if err == io.EOF {
				fmt.Println("EOF")
				break
			}
			if err != nil {
				log.Fatal("Error reading from serial port:", err)
				break
			}

			go p.processBytes(buf[:n])
		}
	}()

	if *dtr {
		log.Println("Set DTR")
		if err := port.SetDTR(true); err != nil {
			log.Fatalf("SetDTR: %v", err)
		}
	}
	if *rts {
		log.Println("Set RTS")
		if err := port.SetRTS(true); err != nil {
			log.Fatalf("SetRTS: %v", err)
		}
	}

	if *sbreak {
		log.Println("Sending BREAK")
		port.Break(10 * time.Millisecond)
	}

	// Wait for HELLO message
	for i := 0; i < 10 && !p.hello; i++ {
		log.Println("Waiting for HELLO message...")
		time.Sleep(1 * time.Second)
	}

	if !p.hello {
		log.Println("No HELLO message received")
		return
	}

	log.Println("Sending STOP command")
	if _, err := port.Write(p.newCommand(CMD_STOP, nil)); err != nil {
		log.Fatalf("Send STOP: %v", err)
		return
	}

	port.Drain()
	time.Sleep(1 * time.Second)

	log.Println("Sending CONFIG command")
	mode := byte(MODE_VHF)
	if *band == "uhf" || *freq >= UHF_MIN_FREQ {
		mode = byte(MODE_UHF)
	}
	if _, err := port.Write(p.newCommand(CMD_CONFIG, []byte{mode})); err != nil {
		log.Fatalf("Send CONFIG: %v", err)
		return
	}

	port.Drain()

	// Wait for VERSION message
	for i := 0; i < 10 && p.version == 0; i++ {
		log.Println("Waiting for VERSION message...")
		time.Sleep(1 * time.Second)
	}

	if *volume < 0.0 {
		*volume = 0.0
	} else if *volume > 1.0 {
		*volume = 1.0
	}
	p.player.SetVolume(*volume)
	p.player.Play()

	log.Println("Sending GROUP command")
	rbw := DRA818_25K
	if *bw != "wide" {
		rbw = DRA818_12K5
	}
	group := Group{
		bw:       byte(rbw),
		freq_tx:  float32(*freq),
		freq_rx:  float32(*freq),
		squelch:  byte(*squelch),
		ctxss_tx: 0x00,
		ctxss_rx: 0x00,
	}

	var buffer [12]byte
	binary.Encode(buffer[:], binary.LittleEndian, group)
	if n, err := port.Write(p.newCommand(CMD_GROUP, buffer[:])); err != nil || n != len(buffer)+7 {
		log.Fatalf("Send GROUP: %v (%v/%v)", err, n, len(buffer))
		return
	}

	port.Drain()

	fmt.Println("Press Ctrl+C to exit")
	time.Sleep(*wait)
}
