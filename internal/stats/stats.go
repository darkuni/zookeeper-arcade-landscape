// Package stats keeps the extra "Zoo Keeper+" numbers: round, jumps, steps and
// how many animals were held in the zoo, per species. They are worked out by
// watching the original program run (a few well-known routines and RAM
// locations) and the player's controls; the game itself is never changed.
package stats

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"sync"

	"zookeeper/internal/board"
)

const MaxSpecies = 8

// Counts are one set of totals (this game, or all games).
type Counts struct {
	Games     int
	Rounds    int
	Jumps     int
	Steps     int
	BestRound int
	Held      [MaxSpecies]int // animals still in the zoo when a round was completed
	Net       [MaxSpecies]int // animals netted and put back in the cage
	BestJump  int             // most points one landing has paid
}

// Program addresses in the original data-CPU ROM that the tracker watches.
const (
	pcNewGame    = 0xb985 // CLR $04b5,U: a player's game state is cleared
	pcGameOver   = 0xc84c // GAME OVER is printed
	pcRoundDone  = 0xc965 // INC rounds-completed (ROUND n COMPLETED)
	pcJumpPoints = 0xb3aa // landing bonus popup: U = its text, about to be drawn
	nvHigh       = 0x261  // first high-score entry in battery RAM (BCD)

	ramRoundsBCD = 0x4b5 // + U: rounds completed, BCD
	ramPlayerPtr = 0x4b1 // U: base of the current player's state
	ramAnimalEnd = 0x4d3 // last animal slot (byte offset, 16 bit)
	ramAnimalTyp = 0x566 // + slot: species << 1
	ramAnimalSt  = 0x5a6 // + slot: 0 = still inside the zoo at the end of the round
)

// stepFrames is how many frames of walking make one "step" in the read-out.
const stepFrames = 8

type Tracker struct {
	Game, Life Counts
	Icons      [MaxSpecies]*image.RGBA
	Active     bool // a real (not attract-mode) game is running
	Round      int  // current round, 1 based
	Dirty      bool // Life or Icons changed and should be saved
	High       int  // the game's own best score (first entry of its high-score table)
	LastJump   int  // points of the latest landing

	frame     int
	lastStart int
	prevBtn   bool
	moves     int
	mu        sync.Mutex
}

func New() *Tracker { return &Tracker{lastStart: -1000} }

func bcd(v byte) int { return int(v>>4)*10 + int(v&15) }

// OnPC is the board hook: it is called before every data CPU instruction.
func (t *Tracker) OnPC(b *board.Board, pc uint16) {
	switch pc {
	case pcNewGame:
		if t.frame-t.lastStart < 600 && !t.Active {
			t.Game = Counts{}
			t.Active = true
			t.Round = 1
			t.Life.Games++
			t.Game.Games = 1
			t.Dirty = true
		}
	case pcGameOver:
		if t.Active {
			t.Active = false
			t.Dirty = true
		}
	case pcJumpPoints:
		if !t.Active {
			return
		}
		pts := jumpPoints(b)
		if pts <= 0 {
			return
		}
		t.LastJump = pts
		if pts > t.Game.BestJump {
			t.Game.BestJump = pts
		}
		if pts > t.Life.BestJump {
			t.Life.BestJump = pts
			t.Dirty = true
		}
	case pcAnimalNetted:
		if !t.Active {
			return
		}
		rd := func(a uint16) byte { return b.Data.Bus.Read(a) }
		off := int(rd(ramCurSlot))<<8 | int(rd(ramCurSlot+1))
		if off < 0 || off > 62 {
			return
		}
		sp := int(rd(uint16(ramAnimalTyp+off))>>1) % MaxSpecies
		t.Game.Net[sp]++
		t.Life.Net[sp]++
		t.Dirty = true
	case pcRoundDone:
		if !t.Active {
			return
		}
		rd := func(a uint16) byte { return b.Data.Bus.Read(a) }
		sp := int(rd(ramAnimalTyp)>>1) % MaxSpecies
		end := int(rd(ramAnimalEnd))<<8 | int(rd(ramAnimalEnd+1))
		held := 0
		for y := 0; y <= end && y < 64; y += 2 {
			if rd(uint16(ramAnimalSt+y)) == 0 {
				held++
			}
		}
		t.Game.Rounds++
		t.Life.Rounds++
		if t.Game.Rounds > t.Life.BestRound {
			t.Life.BestRound = t.Game.Rounds
		}
		t.Game.BestRound = t.Game.Rounds
		t.Game.Held[sp] += held
		t.Life.Held[sp] += held
		t.Dirty = true
	}
}

// Frame is called once per emulated frame with the player's controls (active
// low, as the board sees them), whether the picture is the framed play screen,
// and the 256-wide RGBA frame.
func (t *Tracker) Frame(b *board.Board, joy byte, framed bool, pix []byte) {
	t.frame++
	// the game's high-score table lives in battery RAM: 3 BCD bytes first
	t.High = bcd(b.NVRAM[nvHigh])*10000 + bcd(b.NVRAM[nvHigh+1])*100 + bcd(b.NVRAM[nvHigh+2])
	if joy&(board.JoyStart1|board.JoyStart2) != board.JoyStart1|board.JoyStart2 {
		t.lastStart = t.frame
	}
	btn := joy&board.JoyButton == 0
	if t.Active && framed {
		if btn && !t.prevBtn {
			t.Game.Jumps++
			t.Life.Jumps++
			t.Dirty = true
		}
		if joy&(board.JoyUp|board.JoyDown|board.JoyLeft|board.JoyRight) != board.JoyUp|board.JoyDown|board.JoyLeft|board.JoyRight {
			t.moves++
			if t.moves >= stepFrames {
				t.moves = 0
				t.Game.Steps++
				t.Life.Steps++
			}
		}
	}
	t.prevBtn = btn
	if t.Active {
		u := int(b.Data.Bus.Read(ramPlayerPtr))<<8 | int(b.Data.Bus.Read(ramPlayerPtr+1))
		r := bcd(b.Data.Bus.Read(uint16(u+ramRoundsBCD))) + 1
		if r > 0 && r < 100 {
			t.Round = r
		}
	}
}

// BonusRound is the round after which the next bonus screen is played: every
// second round (the original tests bit 1 of its level counter).
func (t *Tracker) BonusRound() int {
	r := t.Round
	if r%2 == 1 {
		r++
	}
	return r
}

// FindSprite looks for a coloured animal sprite on the round-complete screen
// and returns it with a transparent background.
func FindSprite(pix []byte) *image.RGBA {
	const W, H = 256, 256
	seen := make([]bool, W*H)
	at := func(x, y int) color.RGBA {
		o := (y*W + x) * 4
		return color.RGBA{pix[o], pix[o+1], pix[o+2], 255}
	}
	lit := func(x, y int) bool { c := at(x, y); return c.R|c.G|c.B != 0 }
	var best *image.Rectangle
	bestArea := 0
	for y := 44; y < 228; y++ {
		for x := 12; x < 244; x++ {
			if seen[y*W+x] || !lit(x, y) {
				continue
			}
			r := image.Rect(x, y, x+1, y+1)
			white, n := 0, 0
			stack := [][2]int{{x, y}}
			seen[y*W+x] = true
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				n++
				if c := at(p[0], p[1]); c.R > 200 && c.G > 200 && c.B > 200 {
					white++
				}
				r = r.Union(image.Rect(p[0], p[1], p[0]+1, p[1]+1))
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := p[0]+dx, p[1]+dy
						if nx < 12 || nx >= 244 || ny < 44 || ny >= 228 || seen[ny*W+nx] || !lit(nx, ny) {
							continue
						}
						seen[ny*W+nx] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
			w, h := r.Dx(), r.Dy()
			if w < 8 || w > 26 || h < 8 || h > 26 || white*3 > n {
				continue
			}
			if n > bestArea {
				bestArea = n
				rr := r
				best = &rr
			}
		}
	}
	if best == nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, best.Dx(), best.Dy()))
	for y := best.Min.Y; y < best.Max.Y; y++ {
		for x := best.Min.X; x < best.Max.X; x++ {
			if lit(x, y) {
				out.SetRGBA(x-best.Min.X, y-best.Min.Y, at(x, y))
			}
		}
	}
	return out
}

// ---- saving ------------------------------------------------------------

type saved struct {
	Life  Counts
	IconV int // icons from older builds were cropped differently and are dropped
	Icons [MaxSpecies][]byte
}

const iconVersion = 2

// Load reads totals and learned icons from path (missing file is fine).
func (t *Tracker) Load(path string) {
	d, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s saved
	if json.Unmarshal(d, &s) != nil {
		return
	}
	t.Life = s.Life
	for i, b := range s.Icons {
		if len(b) == 0 || s.IconV != iconVersion {
			continue
		}
		if im, err := png.Decode(bytes.NewReader(b)); err == nil {
			rgba := image.NewRGBA(im.Bounds())
			for y := 0; y < im.Bounds().Dy(); y++ {
				for x := 0; x < im.Bounds().Dx(); x++ {
					rgba.Set(x, y, im.At(x, y))
				}
			}
			t.Icons[i] = rgba
		}
	}
}

// Save writes totals and icons to path.
func (t *Tracker) Save(path string) error {
	s := saved{Life: t.Life, IconV: iconVersion}
	for i := range t.Icons {
		ic := t.Icon(i)
		if ic == nil {
			continue
		}
		var buf bytes.Buffer
		if png.Encode(&buf, ic) == nil {
			s.Icons[i] = buf.Bytes()
		}
	}
	d, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	t.Dirty = false
	return os.WriteFile(path, d, 0o644)
}

// Icon returns the picture of species sp, or nil if it is not known yet.
func (t *Tracker) Icon(sp int) *image.RGBA {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Icons[sp]
}

// SetIcon stores a picture found by Harvest.
func (t *Tracker) SetIcon(sp int, ic *image.RGBA) {
	t.mu.Lock()
	t.Icons[sp] = ic
	t.mu.Unlock()
	t.Dirty = true
}

// MissingIcons reports whether any of the six species still has no picture.
func (t *Tracker) MissingIcons() bool {
	for sp := 0; sp < 6; sp++ {
		if t.Icon(sp) == nil {
			return true
		}
	}
	return false
}

// jumpPoints reads the number the game is about to print as the landing
// bonus: it points U at a string (first byte a width, then characters, the
// last with bit 7 set or a zero) and calls the text routine.
func jumpPoints(b *board.Board) int {
	u := b.Data.U
	n, any := 0, false
	for k := uint16(1); k < 14; k++ {
		c := b.Data.Bus.Read(u + k)
		if c == 0 || c&0x80 != 0 {
			break
		}
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			any = true
		}
	}
	if !any {
		return 0
	}
	return n
}
