// Package m68705 is a Motorola 68705P3 (6805 core) interpreter used for the
// Zoo Keeper coin-handling microcontroller.
package m68705

const (
	cC = 0x01
	cZ = 0x02
	cN = 0x04
	cI = 0x08
	cH = 0x10
)

// MCU is the CPU with its on-chip RAM, timer and three I/O ports.
type MCU struct {
	A, X, CC byte
	SP       uint16
	PC       uint16
	ROM      [0x800]byte
	ram      [0x80]byte // 0x00-0x7f; 0x10-0x7f is RAM, the rest I/O

	latch [3]byte
	ddr   [3]byte
	In    [3]func() byte     // port input pins
	Out   [3]func(v, m byte) // port output (value with undriven bits high, ddr mask)

	timer, tcr byte
	prescaler  uint32
	IRQLine    bool
	irqPending bool
	irqPrev    bool
	pa         byte
	timerIRQ   bool
	Waiting    bool
	Cycles     uint64
	Illegal    func(m *MCU, pc uint16, op byte)
}

func (m *MCU) Reset() {
	m.latch = [3]byte{0xff, 0x00, 0xff}
	m.ddr = [3]byte{}
	m.pa = 0xff
	m.irqPending = false
	m.A, m.X = 0, 0
	m.CC = 0xe8 | cI
	m.SP = 0x7f
	m.timer = 0xff
	m.tcr = 0x7f
	m.PC = m.rd16(0x7fe)
	m.Waiting = false
}

func (m *MCU) out(p int) {
	if m.Out[p] != nil {
		m.Out[p](m.latch[p], m.ddr[p])
	}
}

func (m *MCU) portRead(p int) byte {
	var in byte = 0xff
	if p == 0 {
		in = m.pa
	} else if m.In[p] != nil {
		in = m.In[p]()
	}
	return m.latch[p]&m.ddr[p] | in&^m.ddr[p]
}

func (m *MCU) rd(a uint16) byte {
	a &= 0x7ff
	switch {
	case a < 0x03:
		return m.portRead(int(a))
	case a >= 4 && a <= 6:
		return m.ddr[a-4]
	case a == 8:
		return m.timer
	case a == 9:
		return m.tcr
	case a < 0x10:
		return 0xff
	case a < 0x80:
		return m.ram[a]
	}
	return m.ROM[a]
}

func (m *MCU) wr(a uint16, v byte) {
	a &= 0x7ff
	switch {
	case a < 0x03:
		diff := m.latch[a] ^ v
		m.latch[a] = v
		if diff&m.ddr[a] != 0 {
			m.out(int(a))
		}
	case a >= 4 && a <= 6:
		if m.ddr[a-4] != v {
			m.ddr[a-4] = v
			m.out(int(a - 4))
		}
	case a == 8:
		m.timer = v
	case a == 9:
		m.tcr = m.tcr&0x80 | v&0x7f
		if v&0x08 != 0 {
			m.prescaler = 0
		}
	case a >= 0x10 && a < 0x80:
		m.ram[a] = v
	}
}

func (m *MCU) rd16(a uint16) uint16 { return uint16(m.rd(a))<<8 | uint16(m.rd(a+1)) }

func (m *MCU) fetch() byte {
	v := m.rd(m.PC)
	m.PC = (m.PC + 1) & 0x7ff
	return v
}
func (m *MCU) fetch16() uint16 {
	h := m.fetch()
	return uint16(h)<<8 | uint16(m.fetch())
}

func (m *MCU) push(v byte) {
	m.ram[m.SP&0x7f] = v
	m.SP--
	if m.SP < 0x60 {
		m.SP = 0x7f
	}
}
func (m *MCU) pull() byte {
	m.SP++
	if m.SP > 0x7f {
		m.SP = 0x60
	}
	return m.ram[m.SP&0x7f]
}

func (m *MCU) flag(f byte, on bool) {
	if on {
		m.CC |= f
	} else {
		m.CC &^= f
	}
}
func (m *MCU) nz(v byte) {
	m.flag(cN, v&0x80 != 0)
	m.flag(cZ, v == 0)
}

func (m *MCU) intr(vec uint16) {
	m.push(byte(m.PC))
	m.push(byte(m.PC >> 8))
	m.push(m.X)
	m.push(m.A)
	m.push(m.CC)
	m.CC |= cI
	m.PC = m.rd16(vec)
}

func (m *MCU) tick(cy int) {
	m.Cycles += uint64(cy)
	if m.tcr&0x20 != 0 { // external clock: not modelled
		return
	}
	ps := uint32(1) << (m.tcr & 7)
	for i := 0; i < cy; i++ {
		m.prescaler++
		if m.prescaler >= ps {
			m.prescaler = 0
			m.timer--
			if m.timer == 0xff {
				m.tcr |= 0x80
			}
		}
	}
}

// Step runs one instruction and returns its cycle count.
func (m *MCU) Step() int {
	if m.IRQLine && !m.irqPrev {
		m.irqPending = true
	}
	m.irqPrev = m.IRQLine
	if m.irqPending && m.CC&cI == 0 {
		m.irqPending = false
		m.Waiting = false
		m.intr(0x7fa)
		m.tick(11)
		return 11
	}
	if m.tcr&0x80 != 0 && m.tcr&0x40 == 0 && m.CC&cI == 0 {
		m.Waiting = false
		m.tcr &^= 0x80
		m.intr(0x7f8)
		m.tick(10)
		return 10
	}
	if m.Waiting {
		m.tick(2)
		return 2
	}
	pc0 := m.PC
	op := m.fetch()
	cy := m.exec(op, pc0)
	m.tick(cy)
	return cy
}

func (m *MCU) ill(pc0 uint16, op byte) int {
	if m.Illegal != nil {
		m.Illegal(m, pc0, op)
	}
	return 2
}

func (m *MCU) rmw(n byte, v byte) byte {
	var r byte
	switch n {
	case 0x0:
		r = byte(0 - int(v))
		m.nz(r)
		m.flag(cC, v != 0)
	case 0x3:
		r = ^v
		m.nz(r)
		m.CC |= cC
	case 0x4:
		m.flag(cC, v&1 != 0)
		r = v >> 1
		m.nz(r)
	case 0x6:
		cin := m.CC & cC
		m.flag(cC, v&1 != 0)
		r = v>>1 | cin<<7
		m.nz(r)
	case 0x7:
		m.flag(cC, v&1 != 0)
		r = v>>1 | v&0x80
		m.nz(r)
	case 0x8:
		m.flag(cC, v&0x80 != 0)
		r = v << 1
		m.nz(r)
	case 0x9:
		cin := m.CC & cC
		m.flag(cC, v&0x80 != 0)
		r = v<<1 | cin
		m.nz(r)
	case 0xa:
		r = v - 1
		m.nz(r)
	case 0xc:
		r = v + 1
		m.nz(r)
	case 0xd:
		r = v
		m.nz(r)
	case 0xf:
		r = 0
		m.CC &^= cN
		m.CC |= cZ
	}
	return r
}

func validRMW(n byte) bool {
	switch n {
	case 0, 3, 4, 6, 7, 8, 9, 0xa, 0xc, 0xd, 0xf:
		return true
	}
	return false
}

func (m *MCU) cond(op byte) bool {
	c := m.CC&cC != 0
	z := m.CC&cZ != 0
	switch op & 0xf {
	case 0:
		return true
	case 1:
		return false
	case 2:
		return !c && !z
	case 3:
		return c || z
	case 4:
		return !c
	case 5:
		return c
	case 6:
		return !z
	case 7:
		return z
	case 8:
		return m.CC&cH == 0
	case 9:
		return m.CC&cH != 0
	case 10:
		return m.CC&cN == 0
	case 11:
		return m.CC&cN != 0
	case 12:
		return false // BMC: interrupt mask clear (I flag)
	case 13:
		return false
	case 14:
		return false // BIL: IRQ pin low
	}
	return false
}

func (m *MCU) add(a, b, c byte) byte {
	r := uint16(a) + uint16(b) + uint16(c)
	res := byte(r)
	m.flag(cH, (a^b^res)&0x10 != 0)
	m.flag(cC, r > 0xff)
	m.nz(res)
	return res
}
func (m *MCU) sub(a, b, c byte) byte {
	r := int(a) - int(b) - int(c)
	res := byte(r)
	m.flag(cC, r < 0)
	m.nz(res)
	return res
}

func (m *MCU) exec(op byte, pc0 uint16) int {
	hi := op >> 4
	lo := op & 0xf
	switch hi {
	case 0: // BRSET/BRCLR
		a := uint16(m.fetch())
		o := int8(m.fetch())
		bit := (op >> 1) & 7
		v := m.rd(a)
		set := v&(1<<bit) != 0
		m.flag(cC, set)
		if set == (op&1 == 0) {
			m.PC = (m.PC + uint16(int16(o))) & 0x7ff
		}
		return 5
	case 1: // BSET/BCLR
		a := uint16(m.fetch())
		bit := (op >> 1) & 7
		v := m.rd(a)
		if op&1 == 0 {
			v |= 1 << bit
		} else {
			v &^= 1 << bit
		}
		m.wr(a, v)
		return 5
	case 2:
		o := int8(m.fetch())
		var take bool
		switch lo {
		case 0xc: // BMC: I flag clear
			take = m.CC&cI == 0
		case 0xd: // BMS
			take = m.CC&cI != 0
		case 0xe: // BIL: IRQ pin low
			take = m.IRQLine
		case 0xf: // BIH
			take = !m.IRQLine
		default:
			take = m.cond(op)
		}
		if take {
			m.PC = (m.PC + uint16(int16(o))) & 0x7ff
		}
		return 3
	case 3:
		if !validRMW(lo) {
			return m.ill(pc0, op)
		}
		a := uint16(m.fetch())
		v := m.rmw(lo, m.rd(a))
		if lo != 0xd {
			m.wr(a, v)
		}
		return 5
	case 4, 5:
		if lo == 2 && hi == 4 { // MUL
			r := uint16(m.A) * uint16(m.X)
			m.A, m.X = byte(r>>8), byte(r)
			m.CC &^= cH | cC
			return 11
		}
		if !validRMW(lo) {
			return m.ill(pc0, op)
		}
		if hi == 4 {
			m.A = m.rmw(lo, m.A)
		} else {
			m.X = m.rmw(lo, m.X)
		}
		return 3
	case 6, 7:
		if !validRMW(lo) {
			if lo == 0xe {
				var ea uint16
				if hi == 6 {
					ea = uint16(m.X) + uint16(m.fetch())
					m.PC = ea & 0x7ff
					return 5
				}
				m.PC = uint16(m.X) & 0x7ff
				return 4
			}
			return m.ill(pc0, op)
		}
		var ea uint16
		cy := 6
		if hi == 6 {
			ea = uint16(m.X) + uint16(m.fetch())
			cy = 7
		} else {
			ea = uint16(m.X)
		}
		v := m.rmw(lo, m.rd(ea))
		if lo != 0xd {
			m.wr(ea, v)
		}
		return cy
	case 8:
		switch op {
		case 0x80: // RTI
			m.CC = m.pull() | 0xe0
			m.A = m.pull()
			m.X = m.pull()
			h := m.pull()
			m.PC = (uint16(h)<<8 | uint16(m.pull())) & 0x7ff
			return 9
		case 0x81:
			h := m.pull()
			m.PC = (uint16(h)<<8 | uint16(m.pull())) & 0x7ff
			return 6
		case 0x83:
			m.intr(0x7fc)
			return 10
		case 0x8e, 0x8f:
			m.Waiting = true
			m.CC &^= cI
			return 2
		}
		return m.ill(pc0, op)
	case 9:
		switch op {
		case 0x97:
			m.X = m.A
		case 0x98:
			m.CC &^= cC
		case 0x99:
			m.CC |= cC
		case 0x9a:
			m.CC &^= cI
		case 0x9b:
			m.CC |= cI
		case 0x9c:
			m.SP = 0x7f
		case 0x9d:
		case 0x9f:
			m.A = m.X
		default:
			return m.ill(pc0, op)
		}
		return 2
	}
	// 0xa0-0xff: arithmetic with addressing modes
	var ea uint16
	var cy int
	switch hi {
	case 0xa:
		cy = 2
	case 0xb:
		ea = uint16(m.fetch())
		cy = 4
	case 0xc:
		ea = m.fetch16() & 0x7ff
		cy = 5
	case 0xd:
		ea = uint16(m.X) + m.fetch16()
		cy = 7
	case 0xe:
		ea = uint16(m.X) + uint16(m.fetch())
		cy = 6
	case 0xf:
		ea = uint16(m.X)
		cy = 5
	}
	get := func() byte {
		if hi == 0xa {
			return m.fetch()
		}
		return m.rd(ea)
	}
	switch lo {
	case 0x0:
		m.A = m.sub(m.A, get(), 0)
	case 0x1:
		m.sub(m.A, get(), 0)
	case 0x2:
		m.A = m.sub(m.A, get(), m.CC&cC)
	case 0x3: // CPX
		m.sub(m.X, get(), 0)
	case 0x4:
		m.A &= get()
		m.nz(m.A)
	case 0x5:
		m.nz(m.A & get())
	case 0x6:
		m.A = get()
		m.nz(m.A)
	case 0x7: // STA
		if hi == 0xa {
			return m.ill(pc0, op)
		}
		m.nz(m.A)
		m.wr(ea, m.A)
		return cy + 1
	case 0x8:
		m.A ^= get()
		m.nz(m.A)
	case 0x9:
		m.A = m.add(m.A, get(), m.CC&cC)
	case 0xa:
		m.A |= get()
		m.nz(m.A)
	case 0xb:
		m.A = m.add(m.A, get(), 0)
	case 0xc: // JMP
		if hi == 0xa {
			return m.ill(pc0, op)
		}
		m.PC = ea & 0x7ff
		return cy - 1
	case 0xd: // BSR / JSR
		if hi == 0xa {
			o := int8(m.fetch())
			m.push(byte(m.PC))
			m.push(byte(m.PC >> 8))
			m.PC = (m.PC + uint16(int16(o))) & 0x7ff
			return 8
		}
		m.push(byte(m.PC))
		m.push(byte(m.PC >> 8))
		m.PC = ea & 0x7ff
		return cy + 2
	case 0xe:
		m.X = get()
		m.nz(m.X)
	case 0xf: // STX
		if hi == 0xa {
			return m.ill(pc0, op)
		}
		m.nz(m.X)
		m.wr(ea, m.X)
		return cy + 1
	}
	return cy
}

// PortAIn drives the port A input pins from outside.
func (m *MCU) PortAIn(v byte) {
	m.pa = v
}
