package video

import "testing"

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
