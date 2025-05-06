# kv4p-go
An alternative client for the kv4p HT radio (https://www.kv4p.com/)

This is an attempt to create an alternative client for the **kv4p HT** radio.

The **official** client is at https://github.com/VanceVagell/kv4p-ht

Since the official client is only available for Android devices, this is an attempt to create a client that can be possibly used on other platforms (Raspberry PI, MacOS, iPadOS, maybe iOS, etc.)

## Install

    go mod tidy
    go build

## Usage

    go run cmd/kv4pht/main.go [options]

where options are:

    // serial port
    -dev string
    	Serial device to use (e.g. /dev/ttyUSB0).
      Leave empty to find a serial port with an ESP32 device.

    // general
    -debug
    	Enable debug output

    // radio
    -band string
    	Band (vhf, uhf) (default "vhf")
    -bw string
    	Bandwidth (wide=25k, narrow=12.5k) (default "wide")
    -freq float
    	Frequency in MHz (default 162.4) // San Francisco Bay NOAA Weather channel
    -high
    	high-pass filter (default true)
    -low
    	low-pass filter (default true)
    -pre
    	pre-emphasis filter
    -reset
    	Reset board
    -scan
    	Scan selected band
    -squelch int
    	Squelch level (0-100)
    -volume int
    	Volume (0-100) (default 100)
    -wait duration
    	Receive time before exiting (default 1m0s)
