// Package assets loads the Zoo Keeper ROM set at run time. The ROMs are not
// part of this repository; supply your own dump.
package assets

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ROMs holds the program images.
type ROMs struct {
	Data  [0x8000]byte  // data CPU, mapped at $8000-$ffff
	Video [0x12000]byte // video CPU: $a000-$ffff fixed layout, $10000 = second bank
	Audio [0x3000]byte  // sound CPU, mapped at $d000-$ffff
	MCU   [0x800]byte   // 68705 coin controller
}

type part struct {
	name string
	size int
	put  func(r *ROMs, b []byte)
}

var layout = []part{
	{"za12", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x0000:], b) }},
	{"za13", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x1000:], b) }},
	{"za14", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x2000:], b) }},
	{"za15", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x3000:], b) }},
	{"za16", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x4000:], b) }},
	{"za17", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x5000:], b) }},
	{"za18", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x6000:], b) }},
	{"za19", 0x1000, func(r *ROMs, b []byte) { copy(r.Data[0x7000:], b) }},
	{"za5", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xa000:], b) }},
	{"za6", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xb000:], b) }},
	{"za7", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xc000:], b) }},
	{"za8", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xd000:], b) }},
	{"za9", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xe000:], b) }},
	{"za10", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0xf000:], b) }},
	{"za3", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0x10000:], b) }},
	{"za4", 0x1000, func(r *ROMs, b []byte) { copy(r.Video[0x11000:], b) }},
	{"za25", 0x1000, func(r *ROMs, b []byte) { copy(r.Audio[0x0000:], b) }},
	{"za26", 0x1000, func(r *ROMs, b []byte) { copy(r.Audio[0x1000:], b) }},
	{"za27", 0x1000, func(r *ROMs, b []byte) { copy(r.Audio[0x2000:], b) }},
	{"za_coin", 0x800, func(r *ROMs, b []byte) { copy(r.MCU[:], b) }},
}

// MissingError lists the ROM files that were not found.
type MissingError struct {
	Missing []string
	Sources []string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("missing ROM files: %s", strings.Join(e.Missing, ", "))
}

// canon reduces "za12.u12", "ZA12.BIN" and "za_coin.bin" to a bare key.
func canon(n string) string {
	n = strings.ToLower(filepath.Base(n))
	if n == "za_coin.bin" || strings.HasPrefix(n, "za_coin") {
		return "za_coin"
	}
	if i := strings.IndexByte(n, '.'); i >= 0 {
		n = n[:i]
	}
	return n
}

func collect(path string, files map[string][]byte) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		ents, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range ents {
			p := filepath.Join(path, e.Name())
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
				_ = collect(p, files)
				continue
			}
			b, err := os.ReadFile(p)
			if err == nil {
				add(files, e.Name(), b)
			}
		}
		return nil
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err == nil {
			add(files, f.Name, b)
		}
	}
	return nil
}

func add(files map[string][]byte, name string, b []byte) {
	k := canon(name)
	if _, dup := files[k]; !dup {
		files[k] = b
	}
}

// Load reads the ROM set from the given zips and folders (first hit wins).
func Load(paths ...string) (*ROMs, error) {
	files := map[string][]byte{}
	var srcs []string
	for _, p := range paths {
		if err := collect(p, files); err == nil {
			srcs = append(srcs, p)
		}
	}
	r := &ROMs{}
	var missing []string
	for _, pt := range layout {
		b, ok := files[pt.name]
		if !ok || len(b) < pt.size {
			missing = append(missing, pt.name)
			continue
		}
		pt.put(r, b[:pt.size])
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, &MissingError{Missing: missing, Sources: srcs}
	}
	return r, nil
}
