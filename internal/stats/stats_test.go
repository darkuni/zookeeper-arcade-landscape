package stats

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"
)

func TestBonusRound(t *testing.T) {
	for round, want := range map[int]int{1: 2, 2: 2, 3: 4, 4: 4, 9: 10} {
		tr := New()
		tr.Round = round
		if got := tr.BonusRound(); got != want {
			t.Errorf("round %d: bonus after %d, want %d", round, got, want)
		}
	}
}

func TestNewGameNeedsAStartPress(t *testing.T) {
	tr := New()
	tr.OnPC(nil, pcNewGame) // attract-mode demo: nobody pressed start
	if tr.Active {
		t.Fatal("demo must not count as a game")
	}
	tr.lastStart = tr.frame
	tr.OnPC(nil, pcNewGame)
	if !tr.Active || tr.LifeP[0].Games != 1 {
		t.Fatal("a started game should be tracked")
	}
	tr.OnPC(nil, pcGameOver)
	if tr.Active {
		t.Fatal("game over should end tracking")
	}
}

func TestFindSprite(t *testing.T) {
	pix := make([]byte, 256*256*4)
	set := func(x, y int, c color.RGBA) {
		o := (y*256 + x) * 4
		pix[o], pix[o+1], pix[o+2], pix[o+3] = c.R, c.G, c.B, 255
	}
	for y := 100; y < 110; y++ {
		for x := 50; x < 62; x++ {
			set(x, y, color.RGBA{200, 100, 200, 255})
		}
	}
	// white text must not be taken for an animal
	for y := 150; y < 160; y++ {
		for x := 80; x < 92; x++ {
			set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	ic := FindSprite(pix)
	if ic == nil || ic.Bounds().Dx() != 12 || ic.Bounds().Dy() != 10 {
		t.Fatalf("sprite not found correctly: %v", ic)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	tr := New()
	tr.LifeP[0] = Counts{Games: 3, Rounds: 7, Jumps: 99, Steps: 1234, BestRound: 4}
	tr.LifeP[0].Held[4] = 12
	ic := image.NewRGBA(image.Rect(0, 0, 4, 3))
	ic.SetRGBA(1, 1, color.RGBA{255, 0, 0, 255})
	tr.Icons[4] = ic
	path := filepath.Join(t.TempDir(), "stats.json")
	if err := tr.Save(path); err != nil {
		t.Fatal(err)
	}
	u := New()
	u.Load(path)
	if u.LifeP != tr.LifeP {
		t.Fatalf("totals differ: %+v vs %+v", u.LifeP, tr.LifeP)
	}
	if u.Icons[4] == nil || u.Icons[4].RGBAAt(1, 1) != (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("icon not restored")
	}
}

func TestDrawPanelsDoesNotPanic(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 384, 216))
	tr := New()
	tr.Active, tr.Round = true, 12
	tr.GameP[0].Held[0], tr.LifeP[0].Held[0] = 1234567, 9999999
	tr.Icons[0] = image.NewRGBA(image.Rect(0, 0, 13, 11))
	DrawPanels(dst, tr)
	if num(1234567) != "1,234,567" {
		t.Fatal("number formatting")
	}
}

func TestTwoPlayerSetupAndSeparateTotals(t *testing.T) {
	tr := New()
	tr.lastStart = tr.frame
	tr.OnPC(nil, pcNewGame) // player 1
	if tr.Players != 1 || tr.LifeP[0].Games != 1 || tr.LifeP[1].Games != 0 {
		t.Fatal("one player after the first set-up")
	}
	tr.Cur = 1
	tr.Game().Jumps = 5
	tr.Cur = 0
	if tr.GameP[0].Jumps != 0 || tr.GameP[1].Jumps != 5 {
		t.Fatal("totals must be kept per player")
	}
	tr.Players = 2
	dst := image.NewRGBA(image.Rect(0, 0, 384, 216))
	tr.Cur = 1
	DrawPanels(dst, tr)
}
