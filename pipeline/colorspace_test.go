package pipeline

import (
	"math"
	"testing"
)

func TestColorSpaceRoundTrip(t *testing.T) {
	src := NewTexture(16, 16, 4, SRGB, OrientationYDown)
	for i := range src.Data {
		src.Data[i] = float32(i%251) / 250
	}
	lin := DecodeToLinear(src)
	if lin.ColorSpace != Linear {
		t.Fatal("decoded texture must be marked linear")
	}
	back := EncodeFromLinear(lin, SRGB)
	for i := range src.Data {
		if math.Abs(float64(src.Data[i]-back.Data[i])) > 1e-5 {
			t.Fatalf("round trip error at %d: %v -> %v", i, src.Data[i], back.Data[i])
		}
	}
}

func TestDataMapsNeverGammaTransformed(t *testing.T) {
	data := grayTex(8, 8, func(x, y int) float32 { return float32(x*y) / 64 })
	dec := DecodeToLinear(data) // Linear in, must be untouched
	for i := range data.Data {
		if dec.Data[i] != data.Data[i] {
			t.Fatalf("linear data map altered by decode at %d", i)
		}
	}
	enc := EncodeFromLinear(data, Linear)
	for i := range data.Data {
		if enc.Data[i] != data.Data[i] {
			t.Fatalf("linear data map altered by encode at %d", i)
		}
	}
}

func TestAlphaNotGammaTransformed(t *testing.T) {
	src := NewTexture(2, 2, 4, SRGB, OrientationYDown)
	for i := range src.Data {
		src.Data[i] = 0.25
	}
	lin := DecodeToLinear(src)
	for i := 0; i < 4; i++ {
		if lin.Data[i*4+3] != 0.25 {
			t.Fatalf("alpha channel gamma-transformed: %v", lin.Data[i*4+3])
		}
		if lin.Data[i*4] == 0.25 {
			t.Fatalf("color channel not decoded")
		}
	}
}

func TestSameContentDifferentColorSpacesNormalizesEqual(t *testing.T) {
	// The same image content authored once in linear and once in sRGB must
	// normalize to identical linear pixels.
	lin := NewTexture(8, 8, 3, Linear, OrientationYDown)
	for i := range lin.Data {
		lin.Data[i] = float32(i%97) / 96
	}
	srgb := EncodeFromLinear(lin, SRGB)
	a := DecodeToLinear(lin)
	b := DecodeToLinear(srgb)
	for i := range a.Data {
		if math.Abs(float64(a.Data[i]-b.Data[i])) > 1e-5 {
			t.Fatalf("normalized pixels differ at %d: %v vs %v", i, a.Data[i], b.Data[i])
		}
	}
}
