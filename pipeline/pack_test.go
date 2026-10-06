package pipeline

import (
	"sync"
	"testing"
)

func grayTex(w, h int, v func(x, y int) float32) *Texture {
	t := NewTexture(w, h, 1, Linear, OrientationYDown)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t.Set(x, y, 0, v(x, y))
		}
	}
	return t
}

func TestPackMapping(t *testing.T) {
	a := grayTex(4, 4, func(x, y int) float32 { return 0.1 })
	b := grayTex(4, 4, func(x, y int) float32 { return 0.5 })
	c := grayTex(4, 4, func(x, y int) float32 { return 0.9 })
	out, err := Pack([]*Texture{a, b, c}, PackConfig{
		Sources: []ChannelSource{
			{Input: 0, Channel: 0},               // R = a
			{Input: 1, Channel: 0},               // G = b
			{Input: 2, Channel: 0, Invert: true}, // B = 1-c
			{Input: -1},                          // A = default
		},
		Defaults: []float32{0, 0, 0, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Channels != 4 {
		t.Fatalf("want 4 channels, got %d", out.Channels)
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if out.At(x, y, 0) != 0.1 || out.At(x, y, 1) != 0.5 {
				t.Fatalf("wrong R/G at (%d,%d): %v %v", x, y, out.At(x, y, 0), out.At(x, y, 1))
			}
			if got := out.At(x, y, 2); got < 0.09 || got > 0.11 {
				t.Fatalf("inverted B wrong at (%d,%d): %v", x, y, got)
			}
			if out.At(x, y, 3) != 1 {
				t.Fatalf("default A wrong at (%d,%d): %v", x, y, out.At(x, y, 3))
			}
		}
	}
	if out.ColorSpace != Linear {
		t.Fatal("packed data map must stay linear")
	}
}

func TestPackDeterministicUnderConcurrency(t *testing.T) {
	mk := func() []*Texture {
		return []*Texture{
			grayTex(64, 64, func(x, y int) float32 { return float32(x) / 64 }),
			grayTex(64, 64, func(x, y int) float32 { return float32(y) / 64 }),
			grayTex(64, 64, func(x, y int) float32 { return float32(x+y) / 128 }),
		}
	}
	cfg := PackConfig{Sources: []ChannelSource{
		{Input: 2, Channel: 0},
		{Input: 0, Channel: 0},
		{Input: 1, Channel: 0},
	}}
	ref, err := Pack(mk(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	refBytes, err := EncodeTexture(ref)
	if err != nil {
		t.Fatal(err)
	}
	// Hammer Pack from many goroutines; every result must be byte-identical.
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := Pack(mk(), cfg)
			if err != nil {
				errs <- err.Error()
				return
			}
			b, _ := EncodeTexture(out)
			if string(b) != string(refBytes) {
				errs <- "non-deterministic pack output"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func TestPackRejectsBadInputs(t *testing.T) {
	a := grayTex(4, 4, func(x, y int) float32 { return 0 })
	b := grayTex(8, 4, func(x, y int) float32 { return 0 })
	_, err := Pack([]*Texture{a, b}, PackConfig{Sources: []ChannelSource{{Input: 0}, {Input: 1}}})
	if ve, ok := err.(*ValidationError); !ok || ve.Kind != ErrSizeMismatch {
		t.Fatalf("want size mismatch, got %v", err)
	}
	_, err = Pack([]*Texture{a, a}, PackConfig{Sources: []ChannelSource{{Input: 0, Channel: 3}}})
	if ve, ok := err.(*ValidationError); !ok || ve.Kind != ErrChannelInsufficient {
		t.Fatalf("want channel insufficient, got %v", err)
	}
	_, err = Pack([]*Texture{a, a}, PackConfig{Sources: []ChannelSource{{Input: 5}}})
	if ve, ok := err.(*ValidationError); !ok || ve.Kind != ErrInvalidConfig {
		t.Fatalf("want invalid config, got %v", err)
	}
}
