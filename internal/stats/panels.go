package stats

import (
	"fmt"
	"image"
	"image/color"

	"zookeeper/internal/video"
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
// canvas (384x216) with the Zoo Keeper+ read-outs. The read-outs follow the
// player whose turn it is; both score boxes stay visible so the other
// player's score can always be seen.
func DrawPanels(dst *image.RGBA, t *Tracker) {
	cur := t.Cur
	if !t.Active {
		cur = 0
	}
	game, life := &t.GameP[cur], &t.LifeP[cur]

	// ---- player labels above the two score boxes -------------------------
	boxW := 58
	label := func(text string, x, w int, on bool) {
		c := colDim
		if on {
			c = colHot
		}
		DrawText(dst, x+(w-TextWidth(text, 1))/2, video.HudY-8, text, c, 1)
	}
	label("PLAYER 1", video.P1X, boxW, !t.Active || cur == 0)
	if !t.Active || t.Players > 1 {
		label("PLAYER 2", video.P2X, 57, t.Active && cur == 1)
	}

	// ---- left: this player's game ----------------------------------------
	x, y := 7, 64
	if t.Players > 1 && t.Active {
		DrawText(dst, x, y-8, fmt.Sprintf("PLAYER %d STATS", cur+1), colHot, 1)
	}
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
	row("JUMPS", num(game.Jumps), false, 2)
	row("STEPS", num(game.Steps), false, 2)

	y += 2
	DrawText(dst, x, y, "ALL TIME", colHot, 1)
	y += 8
	for _, kv := range [][2]string{
		{"GAMES", num(life.Games)},
		{"BEST RD", num(life.BestRound)},
		{"JUMPS", num(life.Jumps)},
		{"STEPS", num(life.Steps)},
	} {
		DrawText(dst, x, y, kv[0], colDim, 1)
		DrawText(dst, x+TextWidth(kv[0]+" ", 1), y, kv[1], colValue, 1)
		y += 7
	}

	// ---- right: high score, best jump, then every animal -----------------
	const colX0, colW = 316, 68
	centre := func(txt string, y, scale int, c color.RGBA) {
		for scale > 1 && TextWidth(txt, scale) > colW-4 {
			scale--
		}
		DrawText(dst, colX0+(colW-TextWidth(txt, scale))/2, y, txt, c, scale)
	}
	ry := video.HudY + 26
	centre("HIGH SCORE", ry, 1, colLabel)
	centre(num(t.High), ry+7, 2, colValue)
	ry += 21
	centre("BEST JUMP", ry, 1, colLabel)
	centre(num(life.BestJump), ry+7, 2, colHot)
	centre("GAME "+num(game.BestJump), ry+19, 1, colDim)
	ry += 30

	rx := 322
	DrawText(dst, rx+16, ry, "HELD", colValue, 1)
	DrawText(dst, rx+34, ry, "NET", colHot, 1)
	ry += 8
	for _, sp := range Order {
		if ic := t.Icon(sp); ic != nil {
			blitIcon(dst, ic, rx+(14-ic.Bounds().Dx())/2, ry+(13-ic.Bounds().Dy())/2)
		} else {
			DrawText(dst, rx+5, ry+4, "?", colDim, 1)
		}
		DrawText(dst, rx+16, ry, num(game.Held[sp]), colValue, 1)
		DrawText(dst, rx+34, ry, num(game.Net[sp]), colHot, 1)
		DrawText(dst, rx+16, ry+7, num(life.Held[sp]), colDim, 1)
		DrawText(dst, rx+34, ry+7, num(life.Net[sp]), colDim, 1)
		ry += 18
	}
}
