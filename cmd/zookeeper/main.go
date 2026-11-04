// Zoo Keeper — Taito America's 1982 arcade game, running natively in Go in a
// widescreen layout.
//
// The game needs the original ROM set (zookeep.zip) for its program, graphics
// and sound; none of Taito's data is bundled with this program.
//
// Controls:  arrows move, Z or Space jump, 5 insert coin, 1 / 2 start one / two
// player game, L cycle widescreen / original square / cabinet 4:3,
// F11 toggle full screen, Esc quit. F1-F4 are the cabinet's service switches.
//
// Gamepad: D-pad/left stick move, A or B jump, Select coin, Start 1 player,
// X or Y 2 players, right bumper change view, left bumper Classic / Plus,
// hold Select+Start to quit.
//
// Zoo Keeper+ adds read-outs in the widescreen side panels (round, bonus
// screen, jumps, steps, animals held). P (or the left bumper) switches them
// off; the original square view (L) and the -classic option give the plain
// original game.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"zookeeper/internal/assets"
	"zookeeper/internal/board"
	"zookeeper/internal/stats"
	"zookeeper/internal/video"
)

const sampleRate = 44100

type app struct {
	b      *board.Board
	comp   *video.Composer
	screen *ebiten.Image
	target float64 // emulated time we should have reached, ns
	coin   int
	view   int // 0 widescreen, 1 original square, 2 cabinet 4:3
	pad    padTracker
	ring   *ring
	plus   bool // show the Zoo Keeper+ side panels
	tr     *stats.Tracker
	frames int
	lastNV [0x400]byte
}

// ring is a small sample FIFO between the emulation and the audio device.
type ring struct {
	mu  sync.Mutex
	buf []float32
}

func (r *ring) write(s []float32) {
	r.mu.Lock()
	r.buf = append(r.buf, s...)
	if over := len(r.buf) - sampleRate/4; over > 0 { // never let latency grow past 250 ms
		r.buf = r.buf[over:]
	}
	r.mu.Unlock()
}

// Read streams the FIFO as 16-bit stereo little-endian samples.
func (r *ring) Read(out []byte) (int, error) {
	n := len(out) / 4
	r.mu.Lock()
	for i := 0; i < n; i++ {
		var v float32
		if len(r.buf) > 0 {
			v = r.buf[0]
			r.buf = r.buf[1:]
		}
		x := int16(v * 30000)
		out[4*i], out[4*i+1] = byte(x), byte(uint16(x)>>8)
		out[4*i+2], out[4*i+3] = out[4*i], out[4*i+1]
	}
	r.mu.Unlock()
	return n * 4, nil
}

func (a *app) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyL) {
		a.view = (a.view + 1) % 3
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		a.plus = !a.plus
		a.saveSettings()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	down := ebiten.IsKeyPressed
	pd := a.pad.poll()
	if pd.quit {
		return ebiten.Termination
	}
	if pd.view {
		a.view = (a.view + 1) % 3
	}
	if pd.plus {
		a.plus = !a.plus
		a.saveSettings()
	}
	joy := byte(0xff)
	if down(ebiten.KeyRight) || pd.right {
		joy &^= board.JoyRight
	}
	if down(ebiten.KeyLeft) || pd.left {
		joy &^= board.JoyLeft
	}
	if down(ebiten.KeyUp) || pd.up {
		joy &^= board.JoyUp
	}
	if down(ebiten.KeyDown) || pd.down {
		joy &^= board.JoyDown
	}
	if down(ebiten.KeyZ) || down(ebiten.KeySpace) || pd.fire {
		joy &^= board.JoyButton
	}
	if down(ebiten.Key1) || pd.start1 {
		joy &^= board.JoyStart1
	}
	if down(ebiten.Key2) || pd.start2 {
		joy &^= board.JoyStart2
	}
	if inpututil.IsKeyJustPressed(ebiten.Key5) || pd.coin {
		a.coin = 4
	}
	cp := byte(0xff)
	if a.coin > 0 {
		cp &^= board.Coin1
		a.coin--
	}
	for k, bit := range map[ebiten.Key]byte{ebiten.KeyF1: board.CoinService1, ebiten.KeyF2: board.CoinService2, ebiten.KeyF3: board.CoinService3, ebiten.KeyF4: board.CoinService4} {
		if down(k) {
			cp &^= bit
		}
	}
	a.b.Joy1, a.b.Joy2, a.b.CoinPort = joy, joy, cp

	a.target += 1e9 / 60
	a.b.RunTo(a.target)
	a.ring.write(a.b.DrainSamples())
	a.tr.Frame(a.b, joy, a.comp.Framed(), a.b.Frame[:])
	a.frames++
	if a.frames%600 == 0 {
		a.saveNVRAM()
		a.saveStats()
	}
	return nil
}

func (a *app) Draw(screen *ebiten.Image) {
	img, bg := a.comp.Build(a.b.Frame[:], a.view)
	if a.plus && a.view == 0 && a.comp.Framed() {
		stats.DrawPanels(img, a.tr)
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if a.screen == nil || a.screen.Bounds().Dx() != w || a.screen.Bounds().Dy() != h {
		a.screen = ebiten.NewImage(w, h)
	}
	a.screen.WritePixels(img.Pix)
	screen.Fill(bg)
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	sx, sy := float64(sw)/float64(w), float64(sh)/float64(h)
	k := sx
	if sy < k {
		k = sy
	}
	kx, ky := k, k
	if a.view == 2 {
		// the cabinet's monitor stretched the square bitmap to 4:3
		kx = k * 4 / 3
		if kx*float64(w) > float64(sw) {
			kx = float64(sw) / float64(w)
			ky = kx * 3 / 4
		}
	}
	if a.view != 2 && k >= 1 {
		kx = float64(int(k))
		ky = kx
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(kx, ky)
	op.GeoM.Translate((float64(sw)-kx*float64(w))/2, (float64(sh)-ky*float64(h))/2)
	screen.DrawImage(a.screen, op)
}

func (a *app) Layout(outW, outH int) (int, int) {
	// work in device pixels so the picture can be scaled by whole numbers
	s := ebiten.Monitor().DeviceScaleFactor()
	return int(float64(outW) * s), int(float64(outH) * s)
}

// ---- settings and high scores -----------------------------------------

func nvramFile() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "zookeeper", "nvram.bin")
}

func (a *app) saveNVRAM() {
	if a.b.NVRAM == a.lastNV {
		return
	}
	f := nvramFile()
	if f == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	if os.WriteFile(f, a.b.NVRAM[:], 0o644) == nil {
		a.lastNV = a.b.NVRAM
	}
}

// settings (which view, Classic or Plus) are remembered between runs.
type settings struct {
	View int
	Plus bool
}

func configFile(name string) string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "zookeeper", name)
}

func (a *app) saveSettings() {
	f := configFile("settings.json")
	if f == "" {
		return
	}
	d, _ := json.Marshal(settings{View: a.view, Plus: a.plus})
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	_ = os.WriteFile(f, d, 0o644)
}

func loadSettings() settings {
	s := settings{View: 0, Plus: true}
	if f := configFile("settings.json"); f != "" {
		if d, err := os.ReadFile(f); err == nil {
			_ = json.Unmarshal(d, &s)
		}
	}
	if s.View < 0 || s.View > 2 {
		s.View = 0
	}
	return s
}

func (a *app) saveStats() {
	if f := configFile("stats.json"); f != "" && a.tr.Dirty {
		_ = os.MkdirAll(filepath.Dir(f), 0o755)
		_ = a.tr.Save(f)
	}
}

// ---- ROM lookup ----------------------------------------------------------

// findROMs returns the ROM sets it can find: the paths given on the command
// line, or zookeep.zip (and friends, and a "roms" folder) next to the program,
// in the working directory, or in the home folder.
func findROMs() []string {
	var given []string
	for _, a := range os.Args[1:] {
		if len(a) > 0 && a[0] != '-' {
			given = append(given, a)
		}
	}
	if len(given) > 0 {
		return given
	}
	dirs := []string{"."}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		dirs = append(dirs, d, filepath.Join(d, "roms"), filepath.Join(d, "..", "Resources"), filepath.Join(d, "..", "..", ".."))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, home)
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		for _, n := range []string{"zookeep.zip", "zookeeper.zip", "zookeep2.zip", "zookeep3.zip"} {
			c := filepath.Join(d, n)
			if abs, err := filepath.Abs(c); err == nil {
				c = abs
			}
			if seen[c] {
				continue
			}
			if _, err := os.Stat(c); err == nil {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// errApp shows a problem in the window, because the Windows build has no
// console to print to.
type errApp struct{ msg string }

func (e *errApp) Update() error { return nil }
func (e *errApp) Draw(s *ebiten.Image) {
	ebitenutil.DebugPrintAt(s, e.msg, 6, 6)
}
func (e *errApp) Layout(_, _ int) (int, int) { return 320, 240 }

func showError(msg string) {
	log.Print(msg)
	ebiten.SetWindowTitle("Zoo Keeper - ROM problem")
	ebiten.SetWindowSize(320*3, 240*3)
	_ = ebiten.RunGame(&errApp{msg})
	os.Exit(1)
}

func romProblem(err error, paths []string) string {
	var me *assets.MissingError
	switch {
	case len(paths) == 0:
		return "ROM set not found.\n\nPut zookeep.zip next to\nzookeeper.exe, or drag the zip\nonto the exe.\n\nIt needs the files za3-za10,\nza12-za19, za25-za27 and\nza_coin.bin."
	case errors.As(err, &me):
		m := "Found:\n"
		for _, p := range paths {
			m += "  " + filepath.Base(p) + "\n"
		}
		m += "\nbut these files are missing:\n"
		for i, n := range me.Missing {
			m += " " + n
			if i%6 == 5 {
				m += "\n"
			}
		}
		return m
	}
	return fmt.Sprintf("Could not load the ROMs:\n%v", err)
}

func main() {
	paths := findROMs()
	roms, err := assets.Load(paths...)
	if err != nil {
		showError(romProblem(err, paths))
	}
	var nv [0x400]byte
	have := false
	if f := nvramFile(); f != "" {
		if d, err := os.ReadFile(f); err == nil && len(d) == len(nv) {
			copy(nv[:], d)
			have = true
		}
	}
	b := board.New(roms)
	if !have {
		// first run: initialise the cabinet's settings the way an operator would
		nv = board.FactoryNVRAM(roms)
	}
	b.NVRAM = nv
	b.Reset()
	st := loadSettings()
	for _, arg := range os.Args[1:] {
		if arg == "-classic" || arg == "--classic" {
			// the plain original game: square picture, no extra read-outs
			st.View, st.Plus = 1, false
		}
		if arg == "-plus" || arg == "--plus" {
			st.View, st.Plus = 0, true
		}
	}
	a := &app{
		b:      b,
		comp:   video.NewComposer(),
		ring:   &ring{},
		lastNV: [0x400]byte{},
		view:   st.View,
		plus:   st.Plus,
		tr:     stats.New(),
	}
	if f := configFile("stats.json"); f != "" {
		a.tr.Load(f)
	}
	b.Hook = func(pc uint16) { a.tr.OnPC(b, pc) }
	if a.tr.MissingIcons() {
		// learn what the animals look like from the player's own ROMs
		go stats.Harvest(roms, board.FactoryNVRAM(roms), a.tr.SetIcon)
	}
	if !have {
		a.saveNVRAM()
	} else {
		a.lastNV = nv
	}
	ctx := audio.NewContext(sampleRate)
	player, err := ctx.NewPlayer(a.ring)
	if err != nil {
		log.Fatal(err)
	}
	player.SetBufferSize(80 * time.Millisecond)
	player.Play()
	ebiten.SetWindowTitle("Zoo Keeper")
	ebiten.SetWindowSize(video.LandW*3, video.LandH*3)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)
	err = ebiten.RunGame(a)
	a.saveNVRAM()
	a.saveStats()
	a.saveSettings()
	if err != nil && !errors.Is(err, ebiten.Termination) {
		log.Fatal(err)
	}
}
