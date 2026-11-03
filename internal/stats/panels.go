package stats

import (
	"fmt"
	"image"
	"image/color"
)

var (
	colLabel = color.RGBA{73, 182, 182, 255}
	colValue = color.RGBA{255, 255, 255, 255}
	colHot   = color.RGBA{255, 255, 73, 255}
	colDim   = color.RGBA{110, 110, 140, 255}
)

func num(n int) string {
	s := fmt.Sprint(n)
	// thousands separators keep long totals readable
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func blitIcon(dst *image.RGBA, ic *image.RGBA, x, y int) {
	b := ic.Bounds()
	for yy := 0; yy < b.Dy(); yy++ {
		for xx := 0; xx < b.Dx(); xx++ {
			c := ic.RGBAAt(xx, yy)
			if c.A != 0 && image.Pt(x+xx, y+yy).In(dst.Rect) {
				dst.SetRGBA(x+xx, y+yy, c)
			}
		}
	}
}

// DrawPanels fills the free areas beside the play field of the widescreen
// canvas (384x216) with the Zoo Keeper+ read-outs.
func DrawPanels(dst *image.RGBA, t *Tracker) {
	// ---- left: this game -------------------------------------------------
	x, y := 7, 58
	row := func(label, value string, hot bool, scale int) {
		DrawText(dst, x, y, label, colLabel, 1)
		c := colValue
		if hot {
			c = colHot
		}
		DrawText(dst, x, y+7, value, c, scale)
		y += 8 + 5*scale + 4
	}
	round := t.Round
	if !t.Active || round < 1 {
		round = 1
	}
	row("ROUND", fmt.Sprint(round), false, 2)
	bonus := t.BonusRound()
	if !t.Active {
		bonus = 2
	}
	if bonus == round {
		row("BONUS SCREEN", "THIS ROUND", true, 1)
	} else {
		row("BONUS SCREEN", fmt.Sprintf("AFTER RD %d", bonus), false, 1)
	}
	row("JUMPS", num(t.Game.Jumps), false, 2)
	row("STEPS", num(t.Game.Steps), false, 2)

	y += 4
	DrawText(dst, x, y, "ALL TIME", colHot, 1)
	y += 8
	for _, kv := range [][2]string{
		{"GAMES", num(t.Life.Games)},
		{"BEST RD", num(t.Life.BestRound)},
		{"JUMPS", num(t.Life.Jumps)},
		{"STEPS", num(t.Life.Steps)},
	} {
		DrawText(dst, x, y, kv[0], colDim, 1)
		DrawText(dst, x+TextWidth(kv[0]+" ", 1), y, kv[1], colValue, 1)
		y += 7
	}

	// ---- right: animals held in the zoo ----------------------------------
	rx, ry := 322, 34
	DrawText(dst, rx, ry, "ANIMALS HELD", colLabel, 1)
	ry += 9
	shown := 0
	for i := 0; i < MaxSpecies; i++ {
		if t.Life.Held[i] == 0 && t.Game.Held[i] == 0 {
			continue
		}
		shown++
		if t.Icons[i] != nil {
			ic := t.Icons[i]
			blitIcon(dst, ic, rx+(14-ic.Bounds().Dx())/2, ry+(12-ic.Bounds().Dy())/2)
		} else {
			DrawText(dst, rx+4, ry+3, "?", colDim, 1)
		}
		DrawText(dst, rx+18, ry, num(t.Game.Held[i]), colValue, 2)
		DrawText(dst, rx+18, ry+12, "ALL "+num(t.Life.Held[i]), colDim, 1)
		ry += 22
	}
	if shown == 0 {
		DrawText(dst, rx, ry, "NONE YET", colDim, 1)
	}
}
