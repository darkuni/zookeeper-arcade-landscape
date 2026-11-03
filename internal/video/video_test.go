package video

import (
	"image"
	"testing"
)

func TestUnframedShowsWholeScreen(t *testing.T) {
	c := NewComposer()
	src := make([]byte, SrcW*SrcH*4)
	img, _ := c.Build(src, 0)
	if c.Framed() {
		t.Fatal("blank frame must not look like a play screen")
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("empty image")
	}
}

func TestFrameColourDoesNotMatter(t *testing.T) {
	for _, col := range [][3]byte{{0xff, 0xff, 0x49}, {0xff, 0x49, 0x49}, {0x49, 0xff, 0x49}, {0x49, 0x92, 0xdb}} {
		c := NewComposer()
		src := make([]byte, SrcW*SrcH*4)
		for _, p := range [][2]int{{5, 128}, {250, 128}, {128, 233}, {5, 228}, {250, 100}} {
			o := (p[1]*SrcW + p[0]) * 4
			src[o], src[o+1], src[o+2], src[o+3] = col[0], col[1], col[2], 255
		}
		var img interface{ Bounds() image.Rectangle }
		for i := 0; i < 3; i++ {
			img, _ = c.Build(src, 0)
		}
		if !c.Framed() {
			t.Fatalf("colour %v not recognised as a play screen", col)
		}
		if b := img.Bounds(); b.Dx() != LandW || b.Dy() != LandH {
			t.Fatalf("expected widescreen canvas, got %v", b)
		}
	}
}
