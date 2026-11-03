// Command dev boots the ROMs on the reference interpreter and writes frames.
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"

	"zookeeper/internal/assets"
	"zookeeper/internal/board"
	"zookeeper/internal/m6809"
)

func main() {
	rom, err := assets.Load(os.Args[1])
	if err != nil {
		panic(err)
	}
	frames, _ := strconv.Atoi(os.Args[2])
	every := 30
	if v := os.Getenv("EVERY"); v != "" {
		every, _ = strconv.Atoi(v)
	}
	b := board.New(rom)
	if nv := os.Getenv("NVLOAD"); nv != "" {
		d, _ := os.ReadFile(nv)
		copy(b.NVRAM[:], d)
	}
	b.Reset()
	var ill = map[uint16]int{}
	b.Data.Illegal = func(c *m6809.CPU, pc uint16, op byte) { ill[pc]++ }
	var clog []string
	b.CRTCLog = &clog
	traceFrom, _ := strconv.Atoi(os.Getenv("TRACE"))
	if os.Getenv("TRACE") != "" {
		n := 0
		b.Trace = func(s string) {
			if b.Frames >= traceFrom && n < 80 {
				fmt.Println(b.Frames, s)
				n++
			}
		}
	}
	if os.Getenv("WATCH") != "" {
		watchRun(b, frames)
		return
	}
	pc := map[uint16]int{}
	type press struct {
		at, dur int
		joy     byte
		coin    byte
	}
	var presses []press
	for _, it := range strings.Split(os.Getenv("PRESS"), ",") {
		if it == "" {
			continue
		}
		var name string
		var at, dur int
		fmt.Sscanf(strings.Replace(strings.Replace(it, "@", " ", 1), "+", " ", 1), "%s %d %d", &name, &at, &dur)
		pr := press{at: at, dur: dur}
		switch name {
		case "start1":
			pr.joy = board.JoyStart1
		case "start2":
			pr.joy = board.JoyStart2
		case "up":
			pr.joy = board.JoyUp
		case "down":
			pr.joy = board.JoyDown
		case "left":
			pr.joy = board.JoyLeft
		case "right":
			pr.joy = board.JoyRight
		case "fire":
			pr.joy = board.JoyButton
		case "svc1":
			pr.coin = board.CoinService1
		case "svc2":
			pr.coin = board.CoinService2
		case "svc3":
			pr.coin = board.CoinService3
		case "svc4":
			pr.coin = board.CoinService4
		case "coin":
			pr.coin = board.Coin1
		}
		presses = append(presses, pr)
	}
	var wav []float32
	for f := 0; f < frames; f++ {
		wav = append(wav, b.DrainSamples()...)
		b.Joy1, b.CoinPort = 0xff, 0xff
		if f%50 == 0 {
			fmt.Printf("frame %d nvram valid=%v\n", f, b.NVRAMValid())
		}
		for _, pr := range presses {
			if f >= pr.at && f < pr.at+pr.dur {
				b.Joy1 &^= pr.joy
				b.CoinPort &^= pr.coin
			}
		}
		if os.Getenv("PCHIST") != "" && f > frames-5 {
			for i := 0; i < 60000; i++ {
				c0 := b.Data.Cycles
				p0 := b.Data.PC
				b.Step()
				if b.Data.Cycles != c0 {
					pc[p0]++
				}
			}
		}
		b.RunFrame()
		if f%every == every-1 || f == frames-1 {
			im := image.NewRGBA(image.Rect(0, 0, 256, 256))
			copy(im.Pix, b.Frame[:])
			fh, _ := os.Create(fmt.Sprintf("/tmp/zk_f%04d.png", f))
			png.Encode(fh, im)
			fh.Close()
		}
	}
	if os.Getenv("STACK") != "" {
		c := b.Data
		fmt.Printf("S=%04x stack:", c.S)
		for a := c.S; a < 0x800 && a < c.S+48; a++ {
			fmt.Printf(" %02x", c.Bus.Read(a))
		}
		fmt.Println()
	}
	if os.Getenv("CRTCLOG") != "" {
		for _, l := range clog {
			fmt.Println(l)
		}
	}
	fmt.Printf("crtc regs: % x\n", b.CRTC())
	fmt.Println(b.Timing())
	fmt.Println(b.DebugPIA())
	nzv := 0
	for _, v := range b.VRAM {
		if v != 0 {
			nzv++
		}
	}
	nzf := 0
	for _, v := range b.Frame {
		if v != 0 {
			nzf++
		}
	}
	fmt.Println("vram nonzero", nzv, "frame nonzero", nzf)
	fmt.Println("frames", b.Frames, "time ms", b.Time()/1e6)
	if os.Getenv("NVDUMP") != "" {
		os.WriteFile(os.Getenv("NVDUMP"), b.NVRAM[:], 0o644)
	}
	if w := os.Getenv("WAV"); w != "" {
		wav = append(wav, b.DrainSamples()...)
		writeWAV(w, wav)
	}
	fmt.Printf("data PC=%04x video PC=%04x sound PC=%04x mcu PC=%03x\n", b.Data.PC, b.Video.PC, b.Sound.PC, b.MCU.PC)
	fmt.Println("illegal:", len(ill))
	if len(pc) > 0 {
		type kv struct {
			k uint16
			v int
		}
		var l []kv
		for k, v := range pc {
			l = append(l, kv{k, v})
		}
		for i := 0; i < len(l); i++ {
			for j := i + 1; j < len(l); j++ {
				if l[j].v > l[i].v {
					l[i], l[j] = l[j], l[i]
				}
			}
		}
		for i := 0; i < 25 && i < len(l); i++ {
			fmt.Printf("%04x %d\n", l[i].k, l[i].v)
		}
	}
}
