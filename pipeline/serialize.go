package pipeline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// serializeMagic identifies the deterministic texture encoding.
var serializeMagic = []byte("PTEX")

// EncodeTexture serializes a texture deterministically: identical pixels and
// metadata always produce identical bytes, which is what makes incremental
// and full rebuilds byte-comparable.
func EncodeTexture(t *Texture) ([]byte, error) {
	if t == nil {
		return nil, fmt.Errorf("cannot encode nil texture")
	}
	var buf bytes.Buffer
	buf.Write(serializeMagic)
	buf.WriteByte(1) // format version
	writeU32 := func(v uint32) {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], v)
		buf.Write(b[:])
	}
	writeU32(uint32(t.Width))
	writeU32(uint32(t.Height))
	writeU32(uint32(t.Channels))
	writeU32(uint32(t.ColorSpace))
	writeU32(uint32(t.Orientation))
	var b [4]byte
	for _, f := range t.Data {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(f))
		buf.Write(b[:])
	}
	return buf.Bytes(), nil
}

// DecodeTexture parses bytes produced by EncodeTexture.
func DecodeTexture(b []byte) (*Texture, error) {
	if len(b) < 5+5*4 {
		return nil, fmt.Errorf("texture payload too short: %d bytes", len(b))
	}
	if !bytes.Equal(b[:4], serializeMagic) {
		return nil, fmt.Errorf("bad texture magic")
	}
	if b[4] != 1 {
		return nil, fmt.Errorf("unsupported texture format version %d", b[4])
	}
	r := b[5:]
	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(r)
		r = r[4:]
		return v
	}
	w := int(readU32())
	h := int(readU32())
	ch := int(readU32())
	cs := ColorSpace(readU32())
	o := Orientation(readU32())
	n := w * h * ch
	if n <= 0 || len(r) != n*4 {
		return nil, fmt.Errorf("texture payload size mismatch: want %d floats, have %d bytes", n, len(r))
	}
	t := NewTexture(w, h, ch, cs, o)
	for i := range t.Data {
		t.Data[i] = math.Float32frombits(binary.LittleEndian.Uint32(r[i*4:]))
	}
	return t, nil
}
