package stats

import (
	"image"
	"image/color"
	"sync"

	"zookeeper/internal/assets"
	"zookeeper/internal/board"
)

// Species are shown in the order the game introduces them: round 1 has the
// species with index 4, round 2 index 2, and so on (the original picks the
// animal for a round from a table indexed by its level).
var Order = [6]int{4, 2, 3, 0, 1, 5}

const (
	pcAnimalNetted = 0xc326 // an animal has been netted and sent back to the cage
	pcAnimalSetup  = 0xcb2e // round set-up picks the species from the level counter
	ramLevel2      = 0x4c2  // level counter x 2
	ramCurSlot     = 0x480  // slot of the animal being processed (16 bit)
)

// Harvest learns what every species looks like by running a private copy of
// the machine, one per species: it makes each round start with that species,
// then copies the animal's picture off the screen at a moment when it stands
// clear of the walls. Nothing is stored in the program; pictures are made
// from the player's own ROMs. onIcon is called (from another goroutine) as
// each picture is found.
func Harvest(rom *assets.ROMs, nv [0x400]byte, onIcon func(species int, ic *image.RGBA)) {
	var wg sync.WaitGroup
	for k := range Order {
		wg.Add(1)
		go func(level int) {
			defer wg.Done()
			harvestOne(rom, nv, level, onIcon)
		}(k)
	}
	wg.Wait()
}

func harvestOne(rom *assets.ROMs, nv [0x400]byte, level int, onIcon func(int, *image.RGBA)) {
	want := Order[level]
	b := board.New(rom)
	b.NVRAM = nv
	b.Reset()
	rd := func(a uint16) byte { return b.Data.Bus.Read(a) }
	b.Hook = func(pc uint16) {
		if pc == pcAnimalSetup {
			b.Data.Bus.Write(ramLevel2, byte(2*level))
		}
	}
	for f := 0; f < 3600; f++ {
		b.Joy1, b.CoinPort = 0xff, 0xff
		if f >= 300 && f < 303 {
			b.CoinPort &^= board.Coin1
		}
		if f >= 340 && f < 346 {
			b.Joy1 &^= board.JoyStart1
		}
		b.RunFrame()
		b.DrainSamples()
		if f < 700 || f%3 != 0 {
			continue
		}
		for slot := 0; slot < 5; slot++ {
			if int(rd(uint16(0x566+2*slot))>>1) != want || rd(uint16(0x5a6+2*slot)) != 0 {
				continue
			}
			x, y := int(rd(uint16(0x627+2*slot))), int(rd(uint16(0x626+2*slot)))
			if x == 0 && y == 0 || (abs(x-129) < 28 && abs(y-8-136) < 28) {
				continue // still inside the cage
			}
			if ic := spriteNear(b.Frame[:], x, y-8); ic != nil {
				onIcon(want, ic)
				return
			}
		}
	}
}

// spriteNear returns the isolated coloured blob around (cx, cy), if there is
// exactly a sprite-sized one.
func spriteNear(pix []byte, cx, cy int) *image.RGBA {
	const W = 256
	at := func(x, y int) color.RGBA {
		o := (y*W + x) * 4
		return color.RGBA{pix[o], pix[o+1], pix[o+2], 255}
	}
	lit := func(x, y int) bool {
		if x < 12 || x >= 244 || y < 44 || y >= 228 {
			return false
		}
		c := at(x, y)
		return c.R|c.G|c.B != 0
	}
	// start from the lit pixel nearest the expected centre
	sx, sy, bd := -1, -1, 1<<30
	for y := cy - 7; y <= cy+7; y++ {
		for x := cx - 7; x <= cx+7; x++ {
			if lit(x, y) {
				if d := (x-cx)*(x-cx) + (y-cy)*(y-cy); d < bd {
					sx, sy, bd = x, y, d
				}
			}
		}
	}
	if sx < 0 {
		return nil
	}
	seen := map[[2]int]bool{{sx, sy}: true}
	stack := [][2]int{{sx, sy}}
	r := image.Rect(sx, sy, sx+1, sy+1)
	n := 0
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n++
		r = r.Union(image.Rect(p[0], p[1], p[0]+1, p[1]+1))
		if r.Dx() > 16 || r.Dy() > 16 {
			return nil // joined to a wall or the cage
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				q := [2]int{p[0] + dx, p[1] + dy}
				if !seen[q] && lit(q[0], q[1]) {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
	}
	if r.Dx() < 7 || r.Dy() < 7 || n < 20 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for p := range seen {
		out.SetRGBA(p[0]-r.Min.X, p[1]-r.Min.Y, at(p[0], p[1]))
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
