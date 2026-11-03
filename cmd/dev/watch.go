package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"zookeeper/internal/board"
)

// watch runs until the data CPU reaches one of the PCs in WATCH and prints state.
func watchRun(b *board.Board, maxFrames int) {
	set := map[uint16]bool{}
	for _, s := range strings.Split(os.Getenv("WATCH"), ",") {
		v, _ := strconv.ParseUint(s, 16, 16)
		set[uint16(v)] = true
	}
	n := 0
	var last uint64
	for b.Frames < maxFrames {
		if set[b.Data.PC] && b.Data.Cycles != last {
			last = b.Data.Cycles
			c := b.Data
			fmt.Printf("frame %d data PC=%04x A=%02x B=%02x X=%04x Y=%04x U=%04x S=%04x DP=%02x CC=%02x\n", b.Frames, c.PC, c.A, c.B, c.X, c.Y, c.U, c.S, c.DP, c.CC)
			n++
			if n > 40 {
				return
			}
		}
		b.Step()
	}
}
