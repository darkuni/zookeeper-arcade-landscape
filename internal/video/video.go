// Package video turns the board's 256x256 frame into the pictures the player
// sees: the original square screen, the cabinet's 4:3 shape, or a widescreen
// layout that moves the score read-outs into the side panels.
package video

import (
	"image"
	"image/color"
)

const (
	SrcW = 256
	SrcH = 256
	// the 6845 displays 240 lines
	ShownH = 240
	// widescreen layout
	LandW = 384
	LandH = 216
)

type rect struct{ x, y, w, h int }

// Parts of the in-game picture (which has a yellow frame round it).
var (
	field    = rect{4, 28, 248, 212} // the whole play area including its top and bottom frame
	hudScore = rect{11, 8, 58, 20}   // player 1 score
	hudKeep  = rect{77, 8, 48, 20}   // keepers left (number and icon)
	hudScor2 = rect{188, 8, 57, 20}  // player 2 score
)

var black = color.RGBA{0, 0, 0, 255}

// Composer builds output pictures and remembers whether the game is showing
// its framed play screen.
type Composer struct {
	framed  bool
	pending int
	land    *image.RGBA
	full    *image.RGBA
}

func NewComposer() *Composer {
	return &Composer{
		land: image.NewRGBA(image.Rect(0, 0, LandW, LandH)),
		full: image.NewRGBA(image.Rect(0, 0, SrcW, ShownH)),
	}
}

func at(src []byte, x, y int) color.RGBA {
	o := (y*SrcW + x) * 4
	return color.RGBA{src[o], src[o+1], src[o+2], src[o+3]}
}

// isFramed reports whether the picture has the coloured border of the play
// screen. The border colour changes from round to round, so any one colour
// that is not black and is the same at every sample point counts.
func isFramed(src []byte) bool {
	y := at(src, 5, 128)
	if y.R == 0 && y.G == 0 && y.B == 0 {
		return false
	}
	return at(src, 250, 128) == y && at(src, 128, 233) == y && at(src, 5, 228) == y && at(src, 250, 100) == y
}

func blit(dst *image.RGBA, src []byte, r rect, dx, dy int) {
	for y := 0; y < r.h; y++ {
		so := ((r.y+y)*SrcW + r.x) * 4
		do := dst.PixOffset(dx, dy+y)
		copy(dst.Pix[do:do+r.w*4], src[so:so+r.w*4])
	}
}

func fill(dst *image.RGBA, r rect, c color.RGBA) {
	for y := r.y; y < r.y+r.h; y++ {
		o := dst.PixOffset(r.x, y)
		for x := 0; x < r.w; x++ {
			dst.Pix[o+4*x], dst.Pix[o+4*x+1], dst.Pix[o+4*x+2], dst.Pix[o+4*x+3] = c.R, c.G, c.B, c.A
		}
	}
}

// Framed reports whether the last Build saw the framed play screen.
func (c *Composer) Framed() bool { return c.framed }

// Build returns the picture for view (0 widescreen, otherwise the square
// picture) and the colour to paint the rest of the window with.
func (c *Composer) Build(src []byte, view int) (*image.RGBA, color.RGBA) {
	f := isFramed(src)
	if f != c.framed {
		c.pending++
		if c.pending >= 2 {
			c.framed, c.pending = f, 0
		}
	} else {
		c.pending = 0
	}
	if view == 0 && c.framed && f {
		fill(c.land, rect{0, 0, LandW, LandH}, black)
		fx := (LandW - field.w) / 2
		fy := (LandH - field.h) / 2
		blit(c.land, src, field, fx, fy)
		blit(c.land, src, hudScore, 8, 8)
		blit(c.land, src, hudKeep, 8, 32)
		blit(c.land, src, hudScor2, LandW-8-hudScor2.w, 8)
		return c.land, black
	}
	copy(c.full.Pix, src[:ShownH*SrcW*4])
	return c.full, black
}
