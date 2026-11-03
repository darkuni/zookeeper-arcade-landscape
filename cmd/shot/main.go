// Command shot plays a scripted session headlessly and writes widescreen
// previews: shot <rom.zip> <outdir> [frames] [every]
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"

	"zookeeper/internal/assets"
	"zookeeper/internal/board"
	"zookeeper/internal/video"
)

func main() {
	rom, err := assets.Load(os.Args[1])
	if err != nil {
		panic(err)
	}
	out := os.Args[2]
	frames, every := 2400, 60
	if len(os.Args) > 3 {
		frames, _ = strconv.Atoi(os.Args[3])
	}
	if len(os.Args) > 4 {
		every, _ = strconv.Atoi(os.Args[4])
	}
	os.MkdirAll(out, 0o755)
	b := board.New(rom)
	b.NVRAM = board.FactoryNVRAM(rom)
	b.Reset()
	comp := video.NewComposer()
	for f := 0; f < frames; f++ {
		b.Joy1, b.CoinPort = 0xff, 0xff
		hold := func(at, n int) bool { return f >= at && f < at+n }
		if hold(300, 6) {
			b.CoinPort &^= board.Coin1
		}
		if hold(340, 6) {
			b.Joy1 &^= board.JoyStart1
		}
		// wander about once the game has begun
		if f > 900 {
			switch (f / 90) % 4 {
			case 0:
				b.Joy1 &^= board.JoyRight
			case 2:
				b.Joy1 &^= board.JoyLeft
			}
			if f%70 < 8 {
				b.Joy1 &^= board.JoyButton
			}
		}
		b.RunFrame()
		b.DrainSamples()
		if f%every == every-1 {
			im, _ := comp.Build(b.Frame[:], 0)
			raw := image.NewRGBA(image.Rect(0, 0, 256, 256))
			copy(raw.Pix, b.Frame[:])
			rh, _ := os.Create(fmt.Sprintf("%s/raw%04d.png", out, f))
			png.Encode(rh, raw)
			rh.Close()
			fh, _ := os.Create(fmt.Sprintf("%s/f%04d.png", out, f))
			png.Encode(fh, im)
			fh.Close()
		}
	}
}
