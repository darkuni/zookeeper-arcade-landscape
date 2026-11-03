// Package m6800 is a Motorola 6800/6802 interpreter, used for the Zoo Keeper
// sound CPU.
package m6800

type Bus interface {
	Read(a uint16) byte
	Write(a uint16, v byte)
}

const (
	CC_C = 0x01
	CC_V = 0x02
	CC_Z = 0x04
	CC_N = 0x08
	CC_I = 0x10
	CC_H = 0x20
)

type CPU struct {
	A, B, CC byte
	X, SP    uint16
	PC       uint16
	Bus      Bus
	Cycles   uint64
	IRQ      bool
	NMI      bool
	Waiting  bool
	Illegal  func(c *CPU, pc uint16, op byte)
}

func (c *CPU) rd(a uint16) byte     { return c.Bus.Read(a) }
func (c *CPU) wr(a uint16, v byte)  { c.Bus.Write(a, v) }
func (c *CPU) rd16(a uint16) uint16 { return uint16(c.rd(a))<<8 | uint16(c.rd(a+1)) }
func (c *CPU) wr16(a uint16, v uint16) {
	c.wr(a, byte(v>>8))
	c.wr(a+1, byte(v))
}
func (c *CPU) fetch() byte {
	v := c.rd(c.PC)
	c.PC++
	return v
}
func (c *CPU) fetch16() uint16 {
	h := c.fetch()
	return uint16(h)<<8 | uint16(c.fetch())
}

func (c *CPU) Reset() {
	c.CC |= CC_I | 0xc0
	c.PC = c.rd16(0xfffe)
	c.Waiting = false
}

func (c *CPU) flag(m byte, on bool) {
	if on {
		c.CC |= m
	} else {
		c.CC &^= m
	}
}
func (c *CPU) nz(v byte) {
	c.flag(CC_N, v&0x80 != 0)
	c.flag(CC_Z, v == 0)
}

func (c *CPU) push(v byte) { c.wr(c.SP, v); c.SP-- }
func (c *CPU) pull() byte  { c.SP++; return c.rd(c.SP) }

func (c *CPU) pushState() {
	c.push(byte(c.PC))
	c.push(byte(c.PC >> 8))
	c.push(byte(c.X))
	c.push(byte(c.X >> 8))
	c.push(c.A)
	c.push(c.B)
	c.push(c.CC | 0xc0)
}

func (c *CPU) interrupt(vec uint16) {
	c.pushState()
	c.CC |= CC_I
	c.PC = c.rd16(vec)
}

func (c *CPU) add(a, b, carry byte) byte {
	r := uint16(a) + uint16(b) + uint16(carry)
	res := byte(r)
	c.flag(CC_H, (a^b^res)&0x10 != 0)
	c.flag(CC_C, r > 0xff)
	c.flag(CC_V, (a^res)&(b^res)&0x80 != 0)
	c.nz(res)
	return res
}
func (c *CPU) sub(a, b, carry byte) byte {
	r := int(a) - int(b) - int(carry)
	res := byte(r)
	c.flag(CC_C, r < 0)
	c.flag(CC_V, (a^b)&(a^res)&0x80 != 0)
	c.nz(res)
	return res
}
func (c *CPU) logic(v byte) {
	c.nz(v)
	c.CC &^= CC_V
}

func (c *CPU) rmw(n byte, v byte) byte {
	var r byte
	switch n {
	case 0x0:
		r = byte(0 - int(v))
		c.flag(CC_C, r == 0x80 || (r != 0))
		c.flag(CC_C, r != 0)
		c.flag(CC_V, r == 0x80)
		c.nz(r)
	case 0x3:
		r = ^v
		c.logic(r)
		c.CC |= CC_C
	case 0x4:
		c.flag(CC_C, v&1 != 0)
		r = v >> 1
		c.CC &^= CC_N
		c.flag(CC_Z, r == 0)
		c.flag(CC_V, c.CC&CC_C != 0) // N^C with N=0
	case 0x6:
		cin := c.CC & CC_C
		c.flag(CC_C, v&1 != 0)
		r = v>>1 | cin<<7
		c.nz(r)
		c.flag(CC_V, (c.CC&CC_N != 0) != (c.CC&CC_C != 0))
	case 0x7:
		c.flag(CC_C, v&1 != 0)
		r = v>>1 | v&0x80
		c.nz(r)
		c.flag(CC_V, (c.CC&CC_N != 0) != (c.CC&CC_C != 0))
	case 0x8:
		c.flag(CC_C, v&0x80 != 0)
		r = v << 1
		c.nz(r)
		c.flag(CC_V, (c.CC&CC_N != 0) != (c.CC&CC_C != 0))
	case 0x9:
		cin := c.CC & CC_C
		c.flag(CC_C, v&0x80 != 0)
		r = v<<1 | cin
		c.nz(r)
		c.flag(CC_V, (c.CC&CC_N != 0) != (c.CC&CC_C != 0))
	case 0xa:
		r = v - 1
		c.flag(CC_V, v == 0x80)
		c.nz(r)
	case 0xc:
		r = v + 1
		c.flag(CC_V, v == 0x7f)
		c.nz(r)
	case 0xd:
		r = v
		c.logic(r)
		c.CC &^= CC_C
	case 0xf:
		r = 0
		c.CC &^= CC_N | CC_V | CC_C
		c.CC |= CC_Z
	}
	return r
}

func (c *CPU) cond(op byte) bool {
	n := c.CC&CC_N != 0
	z := c.CC&CC_Z != 0
	v := c.CC&CC_V != 0
	cy := c.CC&CC_C != 0
	switch op & 0xf {
	case 0:
		return true
	case 1:
		return false
	case 2:
		return !cy && !z
	case 3:
		return cy || z
	case 4:
		return !cy
	case 5:
		return cy
	case 6:
		return !z
	case 7:
		return z
	case 8:
		return !v
	case 9:
		return v
	case 10:
		return !n
	case 11:
		return n
	case 12:
		return n == v
	case 13:
		return n != v
	case 14:
		return !z && n == v
	}
	return z || n != v
}

func (c *CPU) daa() {
	cf := byte(0)
	lo := c.A & 0x0f
	hi := c.A & 0xf0
	if lo > 9 || c.CC&CC_H != 0 {
		cf |= 0x06
	}
	if hi > 0x80 && lo > 9 || hi > 0x90 || c.CC&CC_C != 0 {
		cf |= 0x60
	}
	r := uint16(c.A) + uint16(cf)
	c.A = byte(r)
	c.nz(c.A)
	if r&0x100 != 0 {
		c.CC |= CC_C
	}
}

// Step executes one instruction or interrupt entry.
func (c *CPU) Step() int {
	if c.NMI {
		c.NMI = false
		c.Waiting = false
		c.interrupt(0xfffc)
		c.Cycles += 12
		return 12
	}
	if c.IRQ && c.CC&CC_I == 0 {
		if c.Waiting {
			c.Waiting = false
			c.CC |= CC_I
			c.PC = c.rd16(0xfff8)
		} else {
			c.interrupt(0xfff8)
		}
		c.Cycles += 12
		return 12
	}
	if c.Waiting {
		c.Cycles += 2
		return 2
	}
	pc0 := c.PC
	op := c.fetch()
	cy := c.exec(op, pc0)
	c.Cycles += uint64(cy)
	return cy
}

func (c *CPU) ill(pc0 uint16, op byte) int {
	if c.Illegal != nil {
		c.Illegal(c, pc0, op)
	}
	return 2
}

func (c *CPU) exec(op byte, pc0 uint16) int {
	switch {
	case op < 0x20:
		switch op {
		case 0x01:
			return 2
		case 0x06:
			c.CC = c.A&0x3f | 0xc0
			return 2
		case 0x07:
			c.A = c.CC | 0xc0
			return 2
		case 0x08:
			c.X++
			c.flag(CC_Z, c.X == 0)
			return 4
		case 0x09:
			c.X--
			c.flag(CC_Z, c.X == 0)
			return 4
		case 0x0a:
			c.CC &^= CC_V
			return 2
		case 0x0b:
			c.CC |= CC_V
			return 2
		case 0x0c:
			c.CC &^= CC_C
			return 2
		case 0x0d:
			c.CC |= CC_C
			return 2
		case 0x0e:
			c.CC &^= CC_I
			return 2
		case 0x0f:
			c.CC |= CC_I
			return 2
		case 0x10:
			c.A = c.sub(c.A, c.B, 0)
			return 2
		case 0x11:
			c.sub(c.A, c.B, 0)
			return 2
		case 0x16:
			c.B = c.A
			c.logic(c.B)
			return 2
		case 0x17:
			c.A = c.B
			c.logic(c.A)
			return 2
		case 0x19:
			c.daa()
			return 2
		case 0x1b:
			c.A = c.add(c.A, c.B, 0)
			return 2
		}
		return c.ill(pc0, op)
	case op < 0x30:
		if op == 0x21 {
			c.fetch()
			return 4
		}
		o := int8(c.fetch())
		if c.cond(op) {
			c.PC += uint16(int16(o))
		}
		return 4
	case op < 0x40:
		switch op {
		case 0x30:
			c.X = c.SP + 1
			return 4
		case 0x31:
			c.SP++
			return 4
		case 0x32:
			c.A = c.pull()
			return 4
		case 0x33:
			c.B = c.pull()
			return 4
		case 0x34:
			c.SP--
			return 4
		case 0x35:
			c.SP = c.X - 1
			return 4
		case 0x36:
			c.push(c.A)
			return 4
		case 0x37:
			c.push(c.B)
			return 4
		case 0x39:
			h := c.pull()
			c.PC = uint16(h)<<8 | uint16(c.pull())
			return 5
		case 0x3b:
			c.CC = c.pull() | 0xc0
			c.B = c.pull()
			c.A = c.pull()
			xh := c.pull()
			c.X = uint16(xh)<<8 | uint16(c.pull())
			ph := c.pull()
			c.PC = uint16(ph)<<8 | uint16(c.pull())
			return 10
		case 0x3e:
			c.pushState()
			c.Waiting = true
			return 9
		case 0x3f:
			c.pushState()
			c.CC |= CC_I
			c.PC = c.rd16(0xfffa)
			return 12
		}
		return c.ill(pc0, op)
	case op < 0x60:
		reg := &c.A
		if op >= 0x50 {
			reg = &c.B
		}
		n := op & 0xf
		switch n {
		case 0, 3, 4, 6, 7, 8, 9, 0xa, 0xc, 0xd, 0xf:
			*reg = c.rmw(n, *reg)
			return 2
		}
		return c.ill(pc0, op)
	case op < 0x80:
		n := op & 0xf
		var ea uint16
		cy := 7
		if op < 0x70 {
			ea = c.X + uint16(c.fetch())
		} else {
			ea = c.fetch16()
			cy = 6
		}
		switch n {
		case 0xe:
			c.PC = ea
			if op < 0x70 {
				return 4
			}
			return 3
		case 0, 3, 4, 6, 7, 8, 9, 0xa, 0xc, 0xf:
			c.wr(ea, c.rmw(n, c.rd(ea)))
			return cy
		case 0xd:
			c.rmw(n, c.rd(ea))
			return cy - 1 + 0
		}
		return c.ill(pc0, op)
	}
	// 0x80-0xff
	bside := op >= 0xc0
	mode := (op >> 4) & 3
	n := op & 0xf
	var ea uint16
	cyMem := [4]int{2, 3, 5, 4}
	switch mode {
	case 1:
		ea = uint16(c.fetch())
	case 2:
		ea = c.X + uint16(c.fetch())
	case 3:
		ea = c.fetch16()
	}
	cy := cyMem[mode]
	get := func() byte {
		if mode == 0 {
			return c.fetch()
		}
		return c.rd(ea)
	}
	if !bside {
		switch n {
		case 0x0:
			c.A = c.sub(c.A, get(), 0)
		case 0x1:
			c.sub(c.A, get(), 0)
		case 0x2:
			c.A = c.sub(c.A, get(), c.CC&CC_C)
		case 0x4:
			c.A &= get()
			c.logic(c.A)
		case 0x5:
			c.logic(c.A & get())
		case 0x6:
			c.A = get()
			c.logic(c.A)
		case 0x7:
			if mode == 0 {
				return c.ill(pc0, op)
			}
			c.logic(c.A)
			c.wr(ea, c.A)
			return cy + 1
		case 0x8:
			c.A ^= get()
			c.logic(c.A)
		case 0x9:
			c.A = c.add(c.A, get(), c.CC&CC_C)
		case 0xa:
			c.A |= get()
			c.logic(c.A)
		case 0xb:
			c.A = c.add(c.A, get(), 0)
		case 0xc: // CPX
			var v uint16
			if mode == 0 {
				v = c.fetch16()
			} else {
				v = c.rd16(ea)
			}
			r := int(c.X) - int(v)
			c.flag(CC_N, r&0x8000 != 0)
			c.flag(CC_Z, uint16(r) == 0)
			c.flag(CC_V, (c.X^v)&(c.X^uint16(r))&0x8000 != 0)
			return [4]int{3, 4, 6, 5}[mode]
		case 0xd: // BSR / JSR
			switch mode {
			case 0:
				o := int8(c.fetch())
				c.push(byte(c.PC))
				c.push(byte(c.PC >> 8))
				c.PC += uint16(int16(o))
				return 8
			case 2:
				c.push(byte(c.PC))
				c.push(byte(c.PC >> 8))
				c.PC = ea
				return 8
			case 3:
				c.push(byte(c.PC))
				c.push(byte(c.PC >> 8))
				c.PC = ea
				return 9
			}
			return c.ill(pc0, op)
		case 0xe: // LDS
			var v uint16
			if mode == 0 {
				v = c.fetch16()
			} else {
				v = c.rd16(ea)
			}
			c.SP = v
			c.flag(CC_N, v&0x8000 != 0)
			c.flag(CC_Z, v == 0)
			c.CC &^= CC_V
			return [4]int{3, 4, 6, 5}[mode]
		case 0xf: // STS
			if mode == 0 {
				return c.ill(pc0, op)
			}
			c.flag(CC_N, c.SP&0x8000 != 0)
			c.flag(CC_Z, c.SP == 0)
			c.CC &^= CC_V
			c.wr16(ea, c.SP)
			return [4]int{0, 5, 7, 6}[mode]
		default:
			return c.ill(pc0, op)
		}
		return cy
	}
	switch n {
	case 0x0:
		c.B = c.sub(c.B, get(), 0)
	case 0x1:
		c.sub(c.B, get(), 0)
	case 0x2:
		c.B = c.sub(c.B, get(), c.CC&CC_C)
	case 0x4:
		c.B &= get()
		c.logic(c.B)
	case 0x5:
		c.logic(c.B & get())
	case 0x6:
		c.B = get()
		c.logic(c.B)
	case 0x7:
		if mode == 0 {
			return c.ill(pc0, op)
		}
		c.logic(c.B)
		c.wr(ea, c.B)
		return cy + 1
	case 0x8:
		c.B ^= get()
		c.logic(c.B)
	case 0x9:
		c.B = c.add(c.B, get(), c.CC&CC_C)
	case 0xa:
		c.B |= get()
		c.logic(c.B)
	case 0xb:
		c.B = c.add(c.B, get(), 0)
	case 0xe: // LDX
		var v uint16
		if mode == 0 {
			v = c.fetch16()
		} else {
			v = c.rd16(ea)
		}
		c.X = v
		c.flag(CC_N, v&0x8000 != 0)
		c.flag(CC_Z, v == 0)
		c.CC &^= CC_V
		return [4]int{3, 4, 6, 5}[mode]
	case 0xf: // STX
		if mode == 0 {
			return c.ill(pc0, op)
		}
		c.flag(CC_N, c.X&0x8000 != 0)
		c.flag(CC_Z, c.X == 0)
		c.CC &^= CC_V
		c.wr16(ea, c.X)
		return [4]int{0, 5, 7, 6}[mode]
	default:
		return c.ill(pc0, op)
	}
	return cy
}
