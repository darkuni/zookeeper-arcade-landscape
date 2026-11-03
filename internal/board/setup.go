package board

import "zookeeper/internal/assets"

// FactoryNVRAM boots a board with blank battery-backed RAM, lets the game
// stop at its first-run "location programming" screens, and walks through
// them with the service "Test Advance" switch the way an operator would,
// accepting the factory settings. The returned image is what a freshly
// initialised cabinet holds, so the game opens straight in attract mode.
func FactoryNVRAM(r *assets.ROMs) [0x400]byte {
	b := New(r)
	b.Reset()
	for f := 0; f < 1400; f++ {
		b.CoinPort = 0xff
		// 26 presses of Test Advance, 25 frames apart, starting at frame 100
		if f >= 100 && (f-100)%25 < 6 && (f-100)/25 <= 25 {
			b.CoinPort &^= CoinService1
		}
		b.RunFrame()
		b.DrainSamples()
		if f > 900 && b.NVRAMValid() {
			break
		}
	}
	return b.NVRAM
}
