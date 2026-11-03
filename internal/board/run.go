package board

import "math"

var intensityTab = [16]byte{
	0x00, 0x12, 0x24, 0x49,
	0x12, 0x24, 0x49, 0x92,
	0x5b, 0x6d, 0x92, 0xdb,
	0x7f, 0x91, 0xb6, 0xff,
}

// Colour converts a palette RAM byte (RRGGBBII) to RGB.
func Colour(raw byte) (r, g, bl byte) {
	i := raw & 3
	r = intensityTab[(raw>>6&3)<<2|i]
	g = intensityTab[(raw>>4&3)<<2|i]
	bl = intensityTab[(raw>>2&3)<<2|i]
	return
}

// curTime returns the time the board as a whole has reached.
func (b *Board) curTime() float64 {
	t := b.tData
	if b.tVideo < t {
		t = b.tVideo
	}
	if b.tSound < t {
		t = b.tSound
	}
	if b.tMCU < t {
		t = b.tMCU
	}
	return t
}

// vramLine returns the video RAM row shown on display line l: the 6845's
// memory address counter selects the character row and its raster counter
// the line within it (the game starts the display one character row down).
func (b *Board) vramLine(l int) int {
	cols := int(b.crtc[1])
	start := int(b.crtc[12]&0x3f)<<8 | int(b.crtc[13])
	ma := start + (l>>3)*cols
	return (ma>>2)&0xf8 | l&7
}

// scanline is the value the 6845 row/raster counters left in the scanline
// latch at the start of the last displayed line.
func (b *Board) scanline() byte {
	l := int((b.tVideo - b.frameStart) / b.lineNS)
	if l >= b.visLines {
		l = b.visLines - 1
	}
	if l < 0 {
		l = 0
	}
	return byte(b.vramLine(l))
}

// crtcChanged recomputes the display timing from the 6845 registers.
func (b *Board) crtcChanged() {
	r := &b.crtc
	rows := int(r[9]) + 1
	if r[0] == 0 || r[4] == 0 {
		return
	}
	b.lineNS = float64(int(r[0])+1) * 8 * nsPixel
	b.totalLines = (int(r[4])+1)*rows + int(r[5])
	b.visLines = int(r[6]) * rows
	b.vsStart = int(r[7]) * rows
	b.vsLen = int(r[3] >> 4)
	if b.vsLen == 0 {
		b.vsLen = 16
	}
	if b.visLines > 256 {
		b.visLines = 256
	}
}

func (b *Board) renderLine(y int) {
	bank := int(b.paletteBank) << 8
	var rgb [256][4]byte
	var seen [256]bool
	row := b.vramLine(y) << 8
	for x := 0; x < 256; x++ {
		px := b.VRAM[row+x]
		if b.flip {
			px = b.VRAM[(row+x)^0xffff]
		}
		if !seen[px] {
			r, g, bl := Colour(b.palRAM[bank|int(px)])
			rgb[px] = [4]byte{r, g, bl, 255}
			seen[px] = true
		}
		o := (y*256 + x) * 4
		copy(b.working[o:o+4], rgb[px][:])
	}
}

// events processes everything scheduled up to time t.
func (b *Board) events(t float64) {
	for b.nextLine <= t {
		if b.line < b.visLines {
			b.renderLine(b.line)
		}
		b.line++
		b.nextLine += b.lineNS
		// vsync level for the line that is starting
		lvl := b.line >= b.vsStart && b.line < b.vsStart+b.vsLen
		if lvl != b.vsyncLevel {
			b.vsyncLevel = lvl
			b.sndpia[0].CB1(lvl)
		}
		if b.line >= b.totalLines {
			b.line = 0
			b.frameStart = b.nextLine - b.lineNS
			b.Frame = b.working
			b.FrameDone = true
			b.Frames++
		}
	}
	b.flushAudio(t)
}

// Step advances the machine by one instruction on the CPU that is furthest
// behind.
func (b *Board) Step() {
	// pick the CPU with the smallest local time
	which := 0
	t := b.tData
	if b.tVideo < t {
		which, t = 1, b.tVideo
	}
	if b.tSound < t {
		which, t = 2, b.tSound
	}
	if b.tMCU < t {
		which, t = 3, b.tMCU
	}
	switch which {
	case 0:
		b.tData += float64(b.Data.Step()) * nsData
	case 1:
		b.tVideo += float64(b.Video.Step()) * nsData
	case 2:
		b.tSound += float64(b.Sound.Step()) * nsSound
	case 3:
		b.tMCU += float64(b.MCU.Step()) * nsMCU
	}
	b.events(b.curTime())
}

// RunFrame runs until the next frame completes.
func (b *Board) RunFrame() {
	b.FrameDone = false
	for !b.FrameDone {
		b.Step()
	}
}

// ---- audio --------------------------------------------------------------

func (b *Board) gain() float64 {
	side := func(n byte) float64 {
		if n == 0 {
			return 1
		}
		res := [4]float64{22250, 10250, 5850, 3550}
		g := 0.0
		for i := 0; i < 4; i++ {
			if n&(1<<i) != 0 {
				g += 1 / res[i]
			}
		}
		r := 1 / g
		return r / (r + 10000)
	}
	return (side(b.volume>>4) + side(b.volume&15)) / 2
}

func (b *Board) setDAC(v byte) {
	// integrate the previous value up to the sound CPU's present time
	b.integrateTo(b.tSound)
	b.dacValue = v
}

func (b *Board) integrateTo(t float64) {
	for b.dacLast < t {
		end := b.nextSamp
		if end > t {
			end = t
		}
		if end > b.dacLast {
			b.dacAcc += (float64(b.dacValue) - 128) / 128 * b.gain() * (end - b.dacLast)
			b.dacLast = end
		}
		if end >= b.nextSamp {
			s := b.dacAcc / b.SampleNS
			b.dacAcc = 0
			// one-pole high pass (1.5k * 1uF)
			const a = 0.9937
			out := a * (b.hp + s - b.hpPrev)
			b.hp, b.hpPrev = out, s
			b.Samples = append(b.Samples, float32(math.Max(-1, math.Min(1, out))))
			b.nextSamp += b.SampleNS
		}
	}
}

func (b *Board) flushAudio(t float64) {
	if b.nextSamp == 0 {
		b.nextSamp = b.SampleNS
	}
	b.integrateTo(t)
}

// DrainSamples returns and clears the accumulated mono samples (44.1 kHz).
func (b *Board) DrainSamples() []float32 {
	s := b.Samples
	b.Samples = nil
	return s
}

// Time is the emulated time the slowest processor has reached, in ns.
func (b *Board) Time() float64 { return b.curTime() }

// RunTo steps the machine until emulated time t (ns) has been reached.
func (b *Board) RunTo(t float64) {
	for b.curTime() < t {
		b.Step()
	}
}

// NVRAMValid reports whether the battery-backed RAM holds an initialised
// settings block (the game checks for $55AA at $8645).
func (b *Board) NVRAMValid() bool { return b.NVRAM[0x245] == 0x55 && b.NVRAM[0x246] == 0xaa }

// CRTC returns the 6845 register file as the game programmed it.
func (b *Board) CRTC() []byte { return append([]byte(nil), b.crtc[:]...) }

// Timing returns the display timing for diagnostics.
func (b *Board) Timing() (lineNS float64, total, vis, vsStart, vsLen int) {
	return b.lineNS, b.totalLines, b.visLines, b.vsStart, b.vsLen
}

// DebugPIA returns the first sound PIA's registers and the IRQ state.
func (b *Board) DebugPIA() (ora, ddra, cra, orb, ddrb, crb byte, irq bool, vs bool) {
	ora, ddra, cra, orb, ddrb, crb = b.sndpia[0].State()
	return ora, ddra, cra, orb, ddrb, crb, b.sndpia[0].IRQLevel(), b.vsyncLevel
}
