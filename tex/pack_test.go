package tex

import (
	"sync"
	"testing"
)

func grayImage(w, h int, base float32) *Image {
	img := NewImage(w, h, 1, Linear, Direction{OriginTopLeft: true})
	for k := range img.Data {
		img.Data[k] = base + float32(k%7)*0.01
	}
	return img
}

func TestPackMappingIsExact(t *testing.T) {
	a := grayImage(16, 16, 0.1)
	b := grayImage(16, 16, 0.4)
	c := grayImage(16, 16, 0.7)
	cfg := PackConfig{
		Sources: []ChannelSource{
			{ImageIndex: 2, Channel: 0}, // R <- c
			{ImageIndex: 0, Channel: 0}, // G <- a
			{ImageIndex: 1, Channel: 0}, // B <- b
		},
		OutChannels: 4,
		FillValue:   1,
	}
	out, err := PackChannels([]*Image{a, b, c}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out.Channels != 4 || out.Space != Linear {
		t.Fatalf("output geometry/space wrong: %d ch, %v", out.Channels, out.Space)
	}
	for k := 0; k < 16*16; k++ {
		if out.Data[k*4+0] != c.Data[k] || out.Data[k*4+1] != a.Data[k] ||
			out.Data[k*4+2] != b.Data[k] || out.Data[k*4+3] != 1 {
			t.Fatalf("pixel %d mis-mapped: %v %v %v %v",
				k, out.Data[k*4], out.Data[k*4+1], out.Data[k*4+2], out.Data[k*4+3])
		}
	}
}

func TestPackDeterministicUnderConcurrency(t *testing.T) {
	a := grayImage(64, 64, 0.1)
	b := grayImage(64, 64, 0.5)
	cfg := PackConfig{
		Sources:     []ChannelSource{{0, 0}, {1, 0}, {0, 0}},
		OutChannels: 3,
	}
	want, err := PackChannels([]*Image{a, b}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Hammer PackChannels from many goroutines in varying orders; every
	// result must be pixel-identical to the sequential reference.
	var wg sync.WaitGroup
	errs := make(chan string, 32)
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			imgs := []*Image{a, b}
			if g%2 == 1 {
				imgs = []*Image{b, a} // different slice, same content hashes irrelevant here
				cfg2 := PackConfig{
					Sources:     []ChannelSource{{1, 0}, {0, 0}, {1, 0}},
					OutChannels: 3,
				}
				got, err := PackChannels(imgs, cfg2)
				if err != nil {
					errs <- err.Error()
					return
				}
				if !got.Equal(want) {
					errs <- "reordered inputs produced different pixels"
				}
				return
			}
			got, err := PackChannels(imgs, cfg)
			if err != nil {
				errs <- err.Error()
				return
			}
			if !got.Equal(want) {
				errs <- "concurrent pack produced different pixels"
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

func TestPackRejectsBadConfig(t *testing.T) {
	a := grayImage(8, 8, 0.1)
	b := grayImage(4, 8, 0.2) // wrong size
	cases := []struct {
		name string
		imgs []*Image
		cfg  PackConfig
		kind ErrKind
	}{
		{"no inputs", nil, PackConfig{OutChannels: 3}, ErrInvalidConfig},
		{"zero out channels", []*Image{a}, PackConfig{OutChannels: 0}, ErrInvalidConfig},
		{"too many sources", []*Image{a}, PackConfig{OutChannels: 1, Sources: []ChannelSource{{0, 0}, {0, 0}}}, ErrInvalidConfig},
		{"bad image index", []*Image{a}, PackConfig{OutChannels: 1, Sources: []ChannelSource{{5, 0}}}, ErrInvalidConfig},
		{"size mismatch", []*Image{a, b}, PackConfig{OutChannels: 2, Sources: []ChannelSource{{0, 0}, {1, 0}}}, ErrSizeMismatch},
		{"channel out of range", []*Image{a}, PackConfig{OutChannels: 1, Sources: []ChannelSource{{0, 3}}}, ErrChannelMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PackChannels(tc.imgs, tc.cfg)
			if err == nil {
				t.Fatal("want error")
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("want ValidationError, got %T", err)
			}
			if ve.Kind != tc.kind {
				t.Fatalf("want kind %v, got %v (%v)", tc.kind, ve.Kind, err)
			}
		})
	}
}
