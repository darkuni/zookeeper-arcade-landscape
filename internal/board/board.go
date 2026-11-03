// Package board wires the Zoo Keeper (Taito, Qix hardware) circuit board
// together: data CPU, video CPU, sound CPU, coin MCU, PIAs and video RAM.
package board

import (
	"fmt"
	"zookeeper/internal/assets"
	"zookeeper/internal/m6800"
	"zookeeper/internal/m6809"
	"zookeeper/internal/m68705"
	"zookeeper/internal/pia"
)

// Timing (nanoseconds).
const (
	nsData  = 800.0         // 1.25 MHz
	nsSound = 8e3 / 7.3728  // 7.3728 MHz / 8
	nsMCU   = 1000.0        // 4 MHz / 4
	nsPixel = 200.0         // 5 MHz pixel clock
	lineNS  = 328 * nsPixel // 65.6 us
	Lines   = 273           // lines per frame
	frameNS = Lines * lineNS
)

// Input bits for Joy (active low, as on the board).
const (
	JoyUp     = 0x01
	JoyRight  = 0x02
	JoyDown   = 0x04
	JoyLeft   = 0x08
	JoyStart1 = 0x10
	JoyStart2 = 0x20
	JoyButton = 0x40
)

// Coin port bits (active low).
const (
	CoinService1 = 0x01
	CoinService2 = 0x02
	CoinService3 = 0x04
	CoinService4 = 0x08
	Coin1        = 0x10
	Coin2        = 0x20
	Coin3        = 0x40
	CoinTilt     = 0x80
)

// Board is the whole machine.
type Board struct {
	ROM *assets.ROMs

	Data, Video *m6809.CPU
	Sound       *m6800.CPU
	MCU         *m68705.MCU

	shared  [0x400]byte
	dataRAM [0x400]byte
	NVRAM   [0x400]byte
	palRAM  [0x400]byte
	VRAM    [0x10000]byte
	sndRAM  [0x80]byte

	pia    [3]*pia.PIA
	sndpia [3]*pia.PIA

	addrLatch   [2]byte
	paletteBank byte
	videoBank   int
	flip        bool
	crtc        [18]byte
	crtcAddr    byte
	coinCtrl    byte
	mcuPortAOut byte
	scanVideo   func() byte

	// inputs (active low)
	Joy1, Joy2 byte
	CoinPort   byte

	// time keeping
	tData, tVideo, tSound, tMCU float64
	nextLine                    float64
	line                        int
	lineNS                      float64 // duration of a scan line
	totalLines, visLines        int     // lines per frame / lines displayed
	vsStart, vsLen              int     // vertical sync position and width in lines
	frameStart                  float64
	vsyncLevel                  bool

	Frame     [256 * 256 * 4]byte // RGBA of the last completed frame
	working   [256 * 256 * 4]byte
	FrameDone bool
	Frames    int

	// sound
	// Hook, when set, is called with the data CPU's program counter just
	// before each of its instructions (used for statistics, not for emulation).
	Hook func(pc uint16)

	dacValue                 byte
	volume                   byte
	dacLast                  float64
	dacAcc                   float64
	SampleNS                 float64
	nextSamp                 float64
	Samples                  []float32
	hp                       float64
	hpPrev                   float64
	CoinLockout, CoinCounter bool

	Trace   func(string)
	CRTCLog *[]string
}

type dataBus struct{ b *Board }
type videoBus struct{ b *Board }
type soundBus struct{ b *Board }

func New(r *assets.ROMs) *Board {
	b := &Board{ROM: r}
	b.Joy1, b.Joy2, b.CoinPort = 0xff, 0xff, 0xff
	for i := range b.pia {
		b.pia[i] = pia.New()
		b.sndpia[i] = pia.New()
	}
	b.setupPIAs()

	b.Data = &m6809.CPU{Bus: dataBus{b}}
	b.Video = &m6809.CPU{Bus: videoBus{b}}
	b.Sound = &m6800.CPU{Bus: soundBus{b}}
	b.MCU = &m68705.MCU{ROM: r.MCU}
	b.setupMCU()
	b.SampleNS = 1e9 / 44100
	b.dacValue = 128
	b.lineNS, b.totalLines, b.visLines, b.vsStart, b.vsLen = 328*nsPixel, 273, 256, 262, 3
	b.nextLine = b.lineNS
	return b
}

func (b *Board) setupPIAs() {
	p0, p1, p2 := b.pia[0], b.pia[1], b.pia[2]
	p0.OverrideA, p1.OverrideA, p2.OverrideA = 0xff, 0xff, 0xff
	p0.ReadA = func() byte { return b.Joy1 }
	p0.ReadB = func() byte { return b.mcuPortAOut }
	p0.WriteB = func(v byte) { b.MCU.PortAIn(v) }
	p1.ReadA = func() byte { return 0xff }
	p1.ReadB = func() byte { return 0xff }
	p2.ReadA = func() byte { return b.Joy2 }
	p2.ReadB = func() byte { return 0xff }
	p2.WriteB = func(v byte) {
		b.coinCtrl = v
		b.MCU.IRQLine = v&0x04 != 0
	}

	s0, s1 := b.sndpia[0], b.sndpia[1]
	s0.ReadB = func() byte { return 0xff }
	s1.ReadB = func() byte { return 0 }
	s0.WriteA = func(v byte) { s1.PortAIn(v) }
	s0.WriteB = func(v byte) { b.volume = v }
	s0.CA2Out = func(l bool) { s1.CA1(l) }
	s0.CB2Out = func(l bool) { b.flip = l }
	s0.IRQA = func(bool) { b.updateDataIRQ() }
	s0.IRQB = func(bool) { b.updateDataIRQ() }
	s1.WriteA = func(v byte) { s0.PortAIn(v) }
	s1.WriteB = func(v byte) { b.setDAC(v) }
	s1.CA2Out = func(l bool) { s0.CA1(l) }
	s1.IRQA = func(bool) { b.updateSoundIRQ() }
	s1.IRQB = func(bool) { b.updateSoundIRQ() }
}

func (b *Board) updateDataIRQ() {
	b.Data.IRQ = b.sndpia[0].IRQLevel()
}
func (b *Board) updateSoundIRQ() {
	b.Sound.IRQ = b.sndpia[1].IRQLevel()
}

func (b *Board) setupMCU() {
	m := b.MCU
	m.In[0] = nil
	m.In[1] = func() byte {
		c := b.CoinPort
		return c&0x0f | (c&0x80)>>3
	}
	m.In[2] = func() byte {
		return b.coinCtrl&0x08 | (b.CoinPort&0x70)>>4
	}
	m.Out[0] = func(v, mask byte) {
		b.mcuPortAOut = v
		if b.Trace != nil {
			b.Trace(fmt.Sprintf("M %03x porta out %02x mask %02x", b.MCU.PC, v, mask))
		}
	}
	m.Out[1] = func(v, mask byte) {
		v &= mask
		b.CoinLockout = v&0x40 == 0
		b.CoinCounter = v&0x80 != 0
	}
}

// ---- data CPU bus -------------------------------------------------------

func (d dataBus) Read(a uint16) byte {
	b := d.b
	switch {
	case a >= 0x8000:
		return b.ROM.Data[a-0x8000]
	case a < 0x0400:
		return b.shared[a]
	case a < 0x0800:
		return b.dataRAM[a-0x0400]
	case a < 0x0c00:
		return 0
	case a < 0x1000:
		if a&1 == 1 {
			b.Data.FIRQ = false
		} else {
			b.Video.FIRQ = true
		}
		return 0
	case a < 0x1400:
		return b.sndpia[0].Read(byte(a))
	case a < 0x1800:
		v := b.pia[0].Read(byte(a))
		if b.Trace != nil {
			b.Trace(fmt.Sprintf("D %04x pia0 r%d -> %02x", b.Data.PC, a&3, v))
		}
		return v
	case a < 0x1c00:
		return b.pia[1].Read(byte(a))
	case a < 0x2000:
		return b.pia[2].Read(byte(a))
	}
	return 0
}

func (d dataBus) Write(a uint16, v byte) {
	b := d.b
	switch {
	case a >= 0x8000:
	case a < 0x0400:
		b.shared[a] = v
	case a < 0x0800:
		b.dataRAM[a-0x0400] = v
	case a < 0x0c00:
	case a < 0x1000:
		if a&1 == 1 {
			b.Data.FIRQ = false
		} else {
			b.Video.FIRQ = true
		}
	case a < 0x1400:
		if b.Trace != nil {
			b.Trace(fmt.Sprintf("D %04x snd0 w%d <- %02x", b.Data.PC, a&3, v))
		}
		b.sndpia[0].Write(byte(a), v)
	case a < 0x1800:
		if b.Trace != nil {
			b.Trace(fmt.Sprintf("D %04x pia0 w%d <- %02x", b.Data.PC, a&3, v))
		}
		b.pia[0].Write(byte(a), v)
	case a < 0x1c00:
		b.pia[1].Write(byte(a), v)
	case a < 0x2000:
		if b.Trace != nil {
			b.Trace(fmt.Sprintf("D %04x pia2 w%d <- %02x", b.Data.PC, a&3, v))
		}
		b.pia[2].Write(byte(a), v)
	}
}

// ---- video CPU bus ------------------------------------------------------

func (v videoBus) Read(a uint16) byte {
	b := v.b
	switch {
	case a < 0x8000:
		return b.VRAM[int(a)+int(b.addrLatch[0]&0x80)<<8]
	case a < 0x8400:
		return b.shared[a-0x8000]
	case a < 0x8800:
		return b.NVRAM[a-0x8400]
	case a < 0x9000:
		if a >= 0x8c00 {
			if a&1 == 0 {
				b.Data.FIRQ = true
			} else {
				b.Video.FIRQ = false
			}
		}
		return 0
	case a < 0x9400:
		return b.palRAM[a-0x9000]
	case a < 0x9800:
		if a&3 == 0 {
			return b.VRAM[int(b.addrLatch[0])<<8|int(b.addrLatch[1])]
		}
		return 0
	case a < 0x9c00:
		return b.scanline()
	case a < 0xa000:
		if a&1 == 1 && int(b.crtcAddr) < len(b.crtc) {
			return b.crtc[b.crtcAddr]
		}
		return 0
	case a < 0xc000:
		return b.ROM.Video[0xa000+b.videoBank*0x6000+int(a-0xa000)]
	}
	return b.ROM.Video[a]
}

func (v videoBus) Write(a uint16, d byte) {
	b := v.b
	switch {
	case a < 0x8000:
		b.VRAM[int(a)+int(b.addrLatch[0]&0x80)<<8] = d
	case a < 0x8400:
		b.shared[a-0x8000] = d
	case a < 0x8800:
		b.NVRAM[a-0x8400] = d
	case a < 0x9000:
		switch {
		case a < 0x8c00:
			if a&1 == 1 {
				b.videoBank = int(d>>2) & 1
			}
			b.paletteBank = d & 3
		default:
			if a&1 == 0 {
				b.Data.FIRQ = true
			} else {
				b.Video.FIRQ = false
			}
		}
	case a < 0x9400:
		b.palRAM[a-0x9000] = d
	case a < 0x9800:
		switch a & 3 {
		case 0:
			b.VRAM[int(b.addrLatch[0])<<8|int(b.addrLatch[1])] = d
		case 2:
			b.addrLatch[0] = d
		case 3:
			b.addrLatch[1] = d
		}
	case a < 0x9c00:
	case a < 0xa000:
		if a&1 == 0 {
			b.crtcAddr = d & 0x1f
		} else if int(b.crtcAddr) < len(b.crtc) {
			b.crtc[b.crtcAddr] = d
			if b.Trace != nil {
				b.Trace(fmt.Sprintf("V %04x crtc R%d <- %02x", b.Video.PC, b.crtcAddr, d))
			}
			if b.CRTCLog != nil && b.crtcAddr >= 10 && b.crtcAddr <= 13 {
				*b.CRTCLog = append(*b.CRTCLog, fmt.Sprintf("frame %d line %d R%d=%02x", b.Frames, b.line, b.crtcAddr, d))
			}
			b.crtcChanged()
		}
	}
}

// ---- sound CPU bus ------------------------------------------------------

func (s soundBus) Read(a uint16) byte {
	b := s.b
	switch {
	case a < 0x80:
		return b.sndRAM[a]
	case a >= 0xd000:
		return b.ROM.Audio[a-0xd000]
	case a >= 0x4000 && a < 0x8000:
		return b.sndpia[1].Read(byte(a))
	case a >= 0x2000 && a < 0x4000:
		return b.sndpia[2].Read(byte(a))
	}
	return 0
}

func (s soundBus) Write(a uint16, v byte) {
	b := s.b
	switch {
	case a < 0x80:
		b.sndRAM[a] = v
	case a >= 0x4000 && a < 0x8000:
		b.sndpia[1].Write(byte(a), v)
	case a >= 0x2000 && a < 0x4000:
		b.sndpia[2].Write(byte(a), v)
	}
}

// Reset resets every processor.
func (b *Board) Reset() {
	b.Data.Reset()
	b.Video.Reset()
	b.Sound.Reset()
	b.MCU.Reset()
}
