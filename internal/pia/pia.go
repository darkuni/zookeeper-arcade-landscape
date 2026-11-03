// Package pia implements the Motorola 6821 peripheral interface adapter.
package pia

// PIA is one 6821. Hook fields are optional.
type PIA struct {
	ddra, ddrb, ora, orb byte
	cra, crb             byte
	ina, inb             byte
	ca1, ca2, cb1, cb2   bool // input levels
	ca2o, cb2o           bool // output levels when the pins are outputs
	irqa1, irqa2         bool
	irqb1, irqb2         bool
	irqaOut, irqbOut     bool

	// OverrideA/OverrideB: bits whose input value shows through even when
	// the pin is configured as an output.
	OverrideA, OverrideB byte

	ReadA, ReadB   func() byte  // sample the input pins
	WriteA, WriteB func(v byte) // output pins changed (already masked by DDR)
	CA2Out, CB2Out func(level bool)
	IRQA, IRQB     func(level bool)
}

func New() *PIA {
	p := &PIA{ca2o: true, cb2o: true}
	return p
}

func (p *PIA) Reset() {
	*p = PIA{OverrideA: p.OverrideA, OverrideB: p.OverrideB, ReadA: p.ReadA, ReadB: p.ReadB,
		WriteA: p.WriteA, WriteB: p.WriteB, CA2Out: p.CA2Out, CB2Out: p.CB2Out, IRQA: p.IRQA, IRQB: p.IRQB,
		ca2o: true, cb2o: true}
}

func (p *PIA) updateIRQ() {
	a := (p.irqa1 && p.cra&0x01 != 0) || (p.irqa2 && p.cra&0x08 != 0 && p.cra&0x20 == 0)
	b := (p.irqb1 && p.crb&0x01 != 0) || (p.irqb2 && p.crb&0x08 != 0 && p.crb&0x20 == 0)
	if a != p.irqaOut {
		p.irqaOut = a
		if p.IRQA != nil {
			p.IRQA(a)
		}
	}
	if b != p.irqbOut {
		p.irqbOut = b
		if p.IRQB != nil {
			p.IRQB(b)
		}
	}
}

func (p *PIA) setCA2(v bool) {
	if p.ca2o != v {
		p.ca2o = v
		if p.CA2Out != nil {
			p.CA2Out(v)
		}
	}
}
func (p *PIA) setCB2(v bool) {
	if p.cb2o != v {
		p.cb2o = v
		if p.CB2Out != nil {
			p.CB2Out(v)
		}
	}
}

// PortAIn / PortBIn set the externally driven input pins.
func (p *PIA) PortAIn(v byte) { p.ina = v }
func (p *PIA) PortBIn(v byte) { p.inb = v }

func (p *PIA) readA() byte {
	in := p.ina
	if p.ReadA != nil {
		in = p.ReadA()
	}
	out := p.ora & p.ddra
	return out&^p.OverrideA | in&(^p.ddra|p.OverrideA)
}
func (p *PIA) readB() byte {
	in := p.inb
	if p.ReadB != nil {
		in = p.ReadB()
	}
	out := p.orb & p.ddrb
	return out&^p.OverrideB | in&(^p.ddrb|p.OverrideB)
}

// Read reads register r (0-3).
func (p *PIA) Read(r byte) byte {
	switch r & 3 {
	case 0:
		if p.cra&0x04 == 0 {
			return p.ddra
		}
		v := p.readA()
		p.irqa1, p.irqa2 = false, false
		// CA2 read handshake / pulse
		if p.cra&0x30 == 0x20 {
			switch p.cra & 0x38 {
			case 0x20: // handshake: low until CA1 active edge
				p.setCA2(false)
			case 0x28: // pulse
				p.setCA2(false)
				p.setCA2(true)
			}
		}
		p.updateIRQ()
		return v
	case 1:
		v := p.cra & 0x3f
		if p.irqa1 {
			v |= 0x80
		}
		if p.irqa2 && p.cra&0x20 == 0 {
			v |= 0x40
		}
		return v
	case 2:
		if p.crb&0x04 == 0 {
			return p.ddrb
		}
		v := p.readB()
		p.irqb1, p.irqb2 = false, false
		p.updateIRQ()
		return v
	}
	v := p.crb & 0x3f
	if p.irqb1 {
		v |= 0x80
	}
	if p.irqb2 && p.crb&0x20 == 0 {
		v |= 0x40
	}
	return v
}

func (p *PIA) outA() {
	if p.WriteA != nil {
		p.WriteA(p.ora & p.ddra)
	}
}
func (p *PIA) outB() {
	if p.WriteB != nil {
		p.WriteB(p.orb & p.ddrb)
	}
}

// Write writes register r (0-3).
func (p *PIA) Write(r byte, v byte) {
	switch r & 3 {
	case 0:
		if p.cra&0x04 == 0 {
			p.ddra = v
		} else {
			p.ora = v
		}
		p.outA()
	case 1:
		p.cra = v & 0x3f
		if p.cra&0x20 != 0 {
			switch p.cra & 0x38 {
			case 0x30:
				p.setCA2(false)
			case 0x38:
				p.setCA2(true)
			default:
				p.setCA2(true)
			}
		}
		p.updateIRQ()
	case 2:
		if p.crb&0x04 == 0 {
			p.ddrb = v
		} else {
			p.orb = v
			if p.crb&0x30 == 0x20 {
				switch p.crb & 0x38 {
				case 0x20:
					p.setCB2(false)
				case 0x28:
					p.setCB2(false)
					p.setCB2(true)
				}
			}
		}
		p.outB()
	case 3:
		p.crb = v & 0x3f
		if p.crb&0x20 != 0 {
			switch p.crb & 0x38 {
			case 0x30:
				p.setCB2(false)
			default:
				p.setCB2(true)
			}
		}
		p.updateIRQ()
	}
}

// CA1 drives the CA1 input.
func (p *PIA) CA1(level bool) {
	if level != p.ca1 {
		rising := level
		want := p.cra&0x02 != 0
		if rising == want {
			p.irqa1 = true
			if p.cra&0x38 == 0x20 { // handshake: CA2 returns high
				p.setCA2(true)
			}
		}
		p.ca1 = level
		p.updateIRQ()
	}
}

// CA2 drives CA2 when it is an input.
func (p *PIA) CA2(level bool) {
	if level != p.ca2 {
		if p.cra&0x20 == 0 {
			want := p.cra&0x10 != 0
			if level == want {
				p.irqa2 = true
			}
		}
		p.ca2 = level
		p.updateIRQ()
	}
}

func (p *PIA) CB1(level bool) {
	if level != p.cb1 {
		want := p.crb&0x02 != 0
		if level == want {
			p.irqb1 = true
			if p.crb&0x38 == 0x20 {
				p.setCB2(true)
			}
		}
		p.cb1 = level
		p.updateIRQ()
	}
}

func (p *PIA) CB2(level bool) {
	if level != p.cb2 {
		if p.crb&0x20 == 0 {
			want := p.crb&0x10 != 0
			if level == want {
				p.irqb2 = true
			}
		}
		p.cb2 = level
		p.updateIRQ()
	}
}

// Latched output registers, for debugging.
func (p *PIA) State() (ora, ddra, cra, orb, ddrb, crb byte) {
	return p.ora, p.ddra, p.cra, p.orb, p.ddrb, p.crb
}

// IRQLevel is the combined state of the A and B interrupt outputs.
func (p *PIA) IRQLevel() bool { return p.irqaOut || p.irqbOut }

// PortAIn already exists; MCU style helper names below are kept separate.
