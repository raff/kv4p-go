package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"time"

	"go.bug.st/serial"
)

var (
	cmd_prefix = []byte{0xDE, 0xAD, 0xBE, 0xEF}
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
)

func NewCommand(cmd byte, params []byte) []byte {
	l := len(params)
	buffer := make([]byte, 4+1+2+l)
	copy(buffer[:4], cmd_prefix)
	buffer[4] = cmd
	binary.LittleEndian.PutUint16(buffer[5:], uint16(l))
	if l > 0 {
		copy(buffer[7:], params)
	}

	log.Printf("NewCommand %02x: %02x\n", cmd, buffer)
	return buffer
}

type CommandProcessor struct {
	state  int
	cmd    byte
	plen   int
	params []byte

	hello       bool
	version     uint16
	radioStatus byte
	hwver       byte
	windowSize  int
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
	//log.Println("processCommand", p.cmd, p.plen) //, p.params)

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
		log.Printf("Version: %d, rstatus: %02x, hwver: %02x, windowSize: %d\n", p.version, p.radioStatus, p.hwver, p.windowSize)
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
		log.Printf("S-Meter: %d\n", smeter)
	default:
		fmt.Printf("Unknown command %02x: %02x\n", p.cmd, p.params)
	}

	p.state = 0
	p.cmd = 0
	p.plen = 0
	p.params = nil
}

func (p *CommandProcessor) NewCommand(cmd byte, params []byte) []byte {
	l := len(params)
	buffer := make([]byte, 4+1+2+l)
	copy(buffer[:4], cmd_prefix)
	buffer[4] = cmd
	binary.LittleEndian.PutUint16(buffer[5:], uint16(l))
	if l > 0 {
		copy(buffer[7:], params)
	}

	log.Printf("NewCommand %02x: %02x\n", cmd, buffer)
	l = len(buffer)
	if l > p.windowSize {
		log.Printf("Window size exceeded: %d > %d\n", l, p.windowSize)
		time.Sleep(1 * time.Second)
	}
	p.windowSize -= l
	return buffer
}

func main() {
	dev := flag.String("dev", "", "Serial device to use (e.g. /dev/ttyUSB0)")
	baud := flag.Int("baud", 115200, "Baud rate")
	sbreak := flag.Bool("break", false, "Send BREAK signal")
	dtr := flag.Bool("dtr", false, "Set DTR")
	rts := flag.Bool("rts", false, "Set RTS")
	//stop := flag.Bool("stop", false, "Send STOP command")
	//config := flag.Bool("config", false, "Send CONFIG command")
	loglevel := flag.String("loglevel", "debug", "Log level (none, debug, info, warn, error, trace)")
	flag.Parse()

	switch *loglevel {
	case "debug":

	case "info":

	case "warn":

	case "error":

	case "trace":

	case "none":

	}

	if *dev == "" {
		ports, err := serial.GetPortsList()
		if err != nil {
			log.Fatalf("GetPorts: %v", err)
		}
		if len(ports) == 0 {
			fmt.Println("No serial ports found!")
			return
		}
		for i, port := range ports {
			fmt.Printf("port %v: %v\n", i, port)
		}

		return
	}

	mode := &serial.Mode{
		BaudRate: *baud,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}

	port, err := serial.Open(*dev, mode)
	if err != nil {
		log.Fatalf("Open %v: %v", *dev, err)
	}

	defer port.Close()

	p := &CommandProcessor{}

	// Read from the serial port
	go func() {
		buf := make([]byte, 1024)

		for {
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

	//if *stop {
	log.Println("Sending STOP command")
	if _, err := port.Write(NewCommand(CMD_STOP, nil)); err != nil {
		log.Fatalf("Send STOP: %v", err)
		return
	}

	time.Sleep(1 * time.Second)
	//}

	//if *config {
	log.Println("Sending CONFIG command")
	if _, err := port.Write(NewCommand(CMD_CONFIG, []byte{MODE_VHF})); err != nil {
		log.Fatalf("Send CONFIG: %v", err)
		return
	}
	//}

	fmt.Println("Press Ctrl+C to exit")
	time.Sleep(60 * time.Second)
}
