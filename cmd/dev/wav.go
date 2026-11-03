package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
)

func writeWAV(path string, s []float32) {
	f, _ := os.Create(path)
	defer f.Close()
	n := len(s) * 2
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+n))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 44100)
	binary.LittleEndian.PutUint32(h[28:], 88200)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(n))
	f.Write(h)
	buf := make([]byte, n)
	var sum float64
	var peak float64
	for i, v := range s {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(v*32000)))
		sum += float64(v) * float64(v)
		peak = math.Max(peak, math.Abs(float64(v)))
	}
	f.Write(buf)
	fmt.Printf("wav: %d samples, rms %.4f peak %.3f\n", len(s), math.Sqrt(sum/float64(len(s)+1)), peak)
}
