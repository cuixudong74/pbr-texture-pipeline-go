package tex

import (
	"encoding/binary"
	"fmt"
	"math"
)

// imageMagic identifies serialized Image payloads inside cache entries.
const imageMagic = "TXI1"

// EncodeImage serializes an image to a deterministic byte form suitable
// for cache storage: same image content always yields the same bytes.
func EncodeImage(img *Image) []byte {
	out := make([]byte, 0, 4+24+4*len(img.Data))
	out = append(out, imageMagic...)
	var u32 [4]byte
	put := func(v uint32) {
		binary.LittleEndian.PutUint32(u32[:], v)
		out = append(out, u32[:]...)
	}
	put(uint32(img.Width))
	put(uint32(img.Height))
	put(uint32(img.Channels))
	put(uint32(img.Space))
	var flags uint32
	if img.Dir.NormalYUp {
		flags |= 1
	}
	if img.Dir.OriginTopLeft {
		flags |= 2
	}
	put(flags)
	put(uint32(len(img.Data)))
	for _, v := range img.Data {
		put(math.Float32bits(v))
	}
	return out
}

// DecodeImage parses bytes produced by EncodeImage. Truncated or
// inconsistent payloads are rejected, never silently repaired.
func DecodeImage(data []byte) (*Image, error) {
	if len(data) < 4+24 || string(data[:4]) != imageMagic {
		return nil, fmt.Errorf("decode: bad magic or truncated header")
	}
	off := 4
	get := func() uint32 {
		v := binary.LittleEndian.Uint32(data[off:])
		off += 4
		return v
	}
	w := int(get())
	h := int(get())
	ch := int(get())
	space := ColorSpace(get())
	flags := get()
	n := int(get())
	if w <= 0 || h <= 0 || ch <= 0 || n != w*h*ch {
		return nil, fmt.Errorf("decode: inconsistent geometry %dx%dx%d with %d values", w, h, ch, n)
	}
	if len(data) != off+4*n {
		return nil, fmt.Errorf("decode: payload length %d, want %d", len(data)-off, 4*n)
	}
	img := NewImage(w, h, ch, space, Direction{
		NormalYUp:     flags&1 != 0,
		OriginTopLeft: flags&2 != 0,
	})
	for k := range img.Data {
		img.Data[k] = math.Float32frombits(get())
	}
	return img, nil
}
