package tex

import (
	"math"
	"testing"
)

func TestColorSpaceRoundTrip(t *testing.T) {
	// Dense sweep: decode(encode(v)) must stay within tolerance.
	for k := 0; k <= 1000; k++ {
		v := float32(k) / 1000
		got := SRGBToLinear(LinearToSRGB(v))
		if math.Abs(float64(got-v)) > 1e-5 {
			t.Fatalf("linear roundtrip: v=%v got=%v", v, got)
		}
		got2 := LinearToSRGB(SRGBToLinear(v))
		if math.Abs(float64(got2-v)) > 1e-5 {
			t.Fatalf("srgb roundtrip: v=%v got=%v", v, got2)
		}
	}
}

func TestColorSpaceImageRoundTrip(t *testing.T) {
	img := NewImage(16, 16, 3, Linear, Direction{})
	for k := range img.Data {
		img.Data[k] = float32((k*37)%256) / 255
	}
	enc, err := ConvertSpace(img, SRGB, true)
	if err != nil {
		t.Fatal(err)
	}
	if enc.Space != SRGB {
		t.Fatal("metadata not updated to SRGB")
	}
	back, err := ConvertSpace(enc, Linear, true)
	if err != nil {
		t.Fatal(err)
	}
	for k := range img.Data {
		if math.Abs(float64(back.Data[k]-img.Data[k])) > 1e-5 {
			t.Fatalf("pixel %d: roundtrip drift %v -> %v", k, img.Data[k], back.Data[k])
		}
	}
	// Source image must not be mutated by conversion.
	if img.Space != Linear {
		t.Fatal("conversion mutated the source image metadata")
	}
}

func TestDataMapsAreNotGammaTransformed(t *testing.T) {
	img := NewImage(8, 8, 1, Linear, Direction{})
	for k := range img.Data {
		img.Data[k] = float32(k) / 64
	}
	out, err := ConvertSpace(img, SRGB, false) // data map: never gamma
	if err != nil {
		t.Fatal(err)
	}
	for k := range img.Data {
		if out.Data[k] != img.Data[k] {
			t.Fatalf("data map value changed at %d: %v -> %v", k, img.Data[k], out.Data[k])
		}
	}
}

func TestSameContentFromDifferentSpacesNormalizesEqual(t *testing.T) {
	linear := NewImage(8, 8, 3, Linear, Direction{})
	for k := range linear.Data {
		linear.Data[k] = float32((k*13)%251) / 250
	}
	// The same content authored in sRGB.
	srgb := NewImage(8, 8, 3, SRGB, Direction{})
	for k := range srgb.Data {
		srgb.Data[k] = LinearToSRGB(linear.Data[k])
	}
	a, err := ConvertSpace(linear, Linear, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ConvertSpace(srgb, Linear, true)
	if err != nil {
		t.Fatal(err)
	}
	for k := range a.Data {
		if math.Abs(float64(a.Data[k]-b.Data[k])) > 1e-6 {
			t.Fatalf("pixel %d: normalized values differ: %v vs %v", k, a.Data[k], b.Data[k])
		}
	}
}

func TestConvertSpaceRejectsNil(t *testing.T) {
	if _, err := ConvertSpace(nil, Linear, true); err == nil {
		t.Fatal("want error for nil image")
	}
}
