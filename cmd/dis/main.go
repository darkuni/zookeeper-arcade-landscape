// Command dis disassembles a CPU program: dis <rom> data|video|audio <start> <end> [bank]
package main

import (
	"fmt"
	"os"
	"strconv"

	"zookeeper/internal/assets"
	"zookeeper/internal/m6809"
)

func main() {
	rom, err := assets.Load(os.Args[1])
	if err != nil {
		panic(err)
	}
	which := os.Args[2]
	s, _ := strconv.ParseUint(os.Args[3], 16, 16)
	e, _ := strconv.ParseUint(os.Args[4], 16, 17)
	bank := 0
	if len(os.Args) > 5 {
		bank, _ = strconv.Atoi(os.Args[5])
	}
	read := func(a uint16) byte {
		switch which {
		case "data":
			if a >= 0x8000 {
				return rom.Data[a-0x8000]
			}
		case "video":
			if a >= 0xc000 {
				return rom.Video[a]
			}
			if a >= 0xa000 {
				return rom.Video[0xa000+bank*0x6000+int(a-0xa000)]
			}
		case "audio":
			if a >= 0xd000 {
				return rom.Audio[a-0xd000]
			}
		}
		return 0
	}
	pc := uint16(s)
	for int(pc) < int(e) {
		t, n := m6809.Disasm(read, pc)
		raw := ""
		for i := 0; i < n; i++ {
			raw += fmt.Sprintf("%02x ", read(pc+uint16(i)))
		}
		fmt.Printf("%04x  %-12s %s\n", pc, raw, t)
		pc += uint16(n)
	}
}
