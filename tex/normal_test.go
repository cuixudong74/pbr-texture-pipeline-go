package tex

import (
	"math"
	"testing"
)

func makeHeight(w, h int, f func(x, y int) float32) *Image {
	img := NewImage(w, h, 1, Linear, Direction{OriginTopLeft: true})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, 0, f(x, y))
		}
	}
	return img
}

func TestNormalIsUnitVector(t *testing.T) {
	src := makeHeight(32, 32, func(x, y int) float32 {
		return float32(math.Sin(float64(x)*0.7) * math.Cos(float64(y)*0.5))
	})
	for _, m := range []GradientMethod{GradientSobel, GradientCentral} {
		n, err := HeightToNormal(src, NormalConfig{Method: m, Strength: 2.5, YUp: true})
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < n.Height; y++ {
			for x := 0; x < n.Width; x++ {
				nx, ny, nz := DecodeNormal(n, x, y)
				l := math.Sqrt(float64(nx*nx + ny*ny + nz*nz))
				if math.Abs(l-1) > 1e-5 {
					t.Fatalf("method %d pixel (%d,%d): normal length %v", m, x, y, l)
				}
				if nz <= 0 {
					t.Fatalf("method %d pixel (%d,%d): normal points away from viewer (nz=%v)", m, x, y, nz)
				}
			}
		}
	}
}

func TestNormalOrientationConvention(t *testing.T) {
	// Height rises toward +v (up). With OriginTopLeft rows, +v is toward
	// decreasing y, so make height depend on -y.
	src := makeHeight(16, 16, func(x, y int) float32 { return float32(15-y) * 0.1 })
	up, err := HeightToNormal(src, NormalConfig{Method: GradientCentral, Strength: 1, YUp: true})
	if err != nil {
		t.Fatal(err)
	}
	down, err := HeightToNormal(src, NormalConfig{Method: GradientCentral, Strength: 1, YUp: false})
	if err != nil {
		t.Fatal(err)
	}
	// Height rises toward +v, so the surface normal leans toward -v:
	// n = normalize(-dh/dx, -dh/dv, 1). OpenGL green encodes the +v
	// component, hence green < 0 here.
	_, nyUp, _ := DecodeNormal(up, 8, 8)
	if nyUp >= 0 {
		t.Fatalf("YUp: expected green < 0 for height rising toward +v, got %v", nyUp)
	}
	// DirectX output must be the exact mirror of OpenGL output.
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if up.At(x, y, 0) != down.At(x, y, 0) || up.At(x, y, 2) != down.At(x, y, 2) {
				t.Fatalf("(%d,%d): x/z differ between conventions", x, y)
			}
			if math.Abs(float64(up.At(x, y, 1)+down.At(x, y, 1)-1)) > 1e-6 {
				t.Fatalf("(%d,%d): green not mirrored: %v vs %v", x, y, up.At(x, y, 1), down.At(x, y, 1))
			}
		}
	}
	if !up.Dir.NormalYUp || down.Dir.NormalYUp {
		t.Fatal("direction metadata does not match requested convention")
	}
}

func TestNormalEdgeTilingNoSeam(t *testing.T) {
	// Non-periodic ramp: only wrap sampling makes the edges consistent.
	w := 8
	src := makeHeight(w, 4, func(x, y int) float32 { return float32(x) })
	n, err := HeightToNormal(src, NormalConfig{Method: GradientCentral, Strength: 1, YUp: true})
	if err != nil {
		t.Fatal(err)
	}
	// Interior gradient: gx = 1.
	nxIn, _, _ := DecodeNormal(n, 3, 1)
	wantIn := float32(-1) / float32(math.Sqrt(2))
	if math.Abs(float64(nxIn-wantIn)) > 1e-5 {
		t.Fatalf("interior nx=%v, want %v", nxIn, wantIn)
	}
	// Right edge wraps to column 0: gx = (h[0]-h[w-2])/2 = (0-6)/2 = -3.
	nxR, _, _ := DecodeNormal(n, w-1, 1)
	wantR := float32(3) / float32(math.Sqrt(10))
	if math.Abs(float64(nxR-wantR)) > 1e-5 {
		t.Fatalf("right-edge nx=%v, want wrapped %v", nxR, wantR)
	}
	// Left edge wraps to column w-1: gx = (h[1]-h[w-1])/2 = (1-7)/2 = -3.
	nxL, _, _ := DecodeNormal(n, 0, 1)
	if math.Abs(float64(nxL-wantR)) > 1e-5 {
		t.Fatalf("left-edge nx=%v, want wrapped %v", nxL, wantR)
	}
}

func TestNormalTiledOutputIsPeriodic(t *testing.T) {
	// A periodic height field must yield a periodic normal map: tiling it
	// produces no visible seam at the border.
	w, h := 16, 16
	src := makeHeight(w, h, func(x, y int) float32 {
		return float32(math.Sin(2 * math.Pi * float64(x) / float64(w)))
	})
	n, err := HeightToNormal(src, NormalConfig{Method: GradientSobel, Strength: 1, YUp: true})
	if err != nil {
		t.Fatal(err)
	}
	// Compare the normal at the left edge of a tile with the normal one
	// pixel into the next tile (same column of the next period): they must
	// be identical, i.e. column 0 equals column w (which is column 0 of
	// the next tile) and the gradient across the seam is continuous.
	// Concretely: normal[0] must equal what central differences across the
	// seam produce from columns w-1 and 1.
	gx := (src.AtWrapped(1, 0, 0) - src.AtWrapped(-1, 0, 0)) / 2
	want := float32(-gx) / float32(math.Sqrt(float64(gx*gx+1)))
	nx0, _, _ := DecodeNormal(n, 0, 0)
	if math.Abs(float64(nx0-want)) > 1e-4 {
		t.Fatalf("seam pixel nx=%v, want %v (Sobel tolerance)", nx0, want)
	}
}

func TestNormalZeroStrengthIsFlat(t *testing.T) {
	src := makeHeight(8, 8, func(x, y int) float32 { return float32(x*y) * 0.3 })
	n, err := HeightToNormal(src, NormalConfig{Method: GradientSobel, Strength: 0, YUp: true})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < 8*8; k++ {
		if n.Data[k*3] != 0.5 || n.Data[k*3+1] != 0.5 || n.Data[k*3+2] != 1 {
			t.Fatalf("pixel %d: want flat normal (0.5,0.5,1), got %v %v %v",
				k, n.Data[k*3], n.Data[k*3+1], n.Data[k*3+2])
		}
	}
}

func TestNormalRejectsBadInput(t *testing.T) {
	if _, err := HeightToNormal(nil, NormalConfig{}); err == nil {
		t.Fatal("nil source: want error")
	}
	src := makeHeight(4, 4, func(x, y int) float32 { return 0 })
	if _, err := HeightToNormal(src, NormalConfig{Strength: -1}); err == nil {
		t.Fatal("negative strength: want error")
	}
}
