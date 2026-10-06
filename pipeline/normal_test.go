package pipeline

import (
	"math"
	"testing"
)

// heightFunc builds a single-channel linear height texture from f(x,y).
func heightFunc(w, h int, f func(x, y int) float32) *Texture {
	t := NewTexture(w, h, 1, Linear, OrientationYDown)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t.Set(x, y, 0, f(x, y))
		}
	}
	return t
}

func decodeNormal(t *Texture, x, y int) (nx, ny, nz float32) {
	return t.At(x, y, 0)*2 - 1, t.At(x, y, 1)*2 - 1, t.At(x, y, 2)*2 - 1
}

func TestNormalIsUnitVector(t *testing.T) {
	src := heightFunc(32, 32, func(x, y int) float32 {
		return float32(math.Sin(float64(x)*0.7) * math.Cos(float64(y)*0.5))
	})
	n, err := GenerateNormal(src, NormalConfig{Strength: 4, Filter: Sobel, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true})
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < n.Height; y++ {
		for x := 0; x < n.Width; x++ {
			nx, ny, nz := decodeNormal(n, x, y)
			l := math.Sqrt(float64(nx*nx + ny*ny + nz*nz))
			if math.Abs(l-1) > 1e-4 {
				t.Fatalf("normal at (%d,%d) not unit: len=%v", x, y, l)
			}
			if nz <= 0 {
				t.Fatalf("normal at (%d,%d) points away from viewer: nz=%v", x, y, nz)
			}
		}
	}
}

func TestNormalConventionFlipsGreen(t *testing.T) {
	// Height increases along image-y (downward).
	src := heightFunc(16, 16, func(x, y int) float32 { return float32(y) / 16 })
	cfg := NormalConfig{Strength: 2, Filter: CentralDifference, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true}
	gl, err := GenerateNormal(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Convention = DirectXNormals
	dx, err := GenerateNormal(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			gxr, gyr, gzr := decodeNormal(gl, x, y)
			dxr, dyr, dzr := decodeNormal(dx, x, y)
			if math.Abs(float64(gxr-dxr)) > 1e-5 || math.Abs(float64(gzr-dzr)) > 1e-5 {
				t.Fatalf("x/z differ between conventions at (%d,%d)", x, y)
			}
			if math.Abs(float64(gyr+dyr)) > 1e-5 {
				t.Fatalf("green channel not flipped: gl=%v dx=%v", gyr, dyr)
			}
		}
	}
	// Sanity: for OpenGL, height rising downward => green points up (+Y).
	_, gy, _ := decodeNormal(gl, 8, 8)
	if gy <= 0 {
		t.Fatalf("OpenGL convention: expected +Y green, got %v", gy)
	}
}

func TestNormalTilingNoSeam(t *testing.T) {
	// Definition of "no seam": the normal at every pixel of the wrapped map
	// must equal the normal at the corresponding interior pixel of a 3x3
	// tiling of the same height map.
	w, h := 32, 16
	f := func(x, y int) float32 {
		return float32(math.Sin(float64(x)*2*math.Pi/float64(w)) * math.Cos(float64(y)*2*math.Pi/float64(h)))
	}
	src := heightFunc(w, h, f)
	tiled := heightFunc(3*w, 3*h, func(x, y int) float32 { return f(x%w, y%h) })
	cfg := NormalConfig{Strength: 2, Filter: Sobel, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true}
	n, err := GenerateNormal(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := GenerateNormal(tiled, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 3; c++ {
				got := n.At(x, y, c)
				want := ref.At(x+w, y+h, c)
				if math.Abs(float64(got-want)) > 1e-6 {
					t.Fatalf("seam at (%d,%d) ch%d: got %v want %v", x, y, c, got, want)
				}
			}
		}
	}
	// Without wrapping, a non-periodic height field must produce edge
	// normals that differ from the tiled reference (i.e. seams).
	src2 := heightFunc(w, h, func(x, y int) float32 {
		return float32(math.Sin(float64(x)*0.7) * math.Cos(float64(y)*0.9))
	})
	tiled2 := heightFunc(3*w, 3*h, func(x, y int) float32 {
		return float32(math.Sin(float64(x%w)*0.7) * math.Cos(float64(y%h)*0.9))
	})
	cfgNoWrap := cfg
	cfgNoWrap.WrapHorizontal = false
	cfgNoWrap.WrapVertical = false
	n2, err := GenerateNormal(src2, cfgNoWrap)
	if err != nil {
		t.Fatal(err)
	}
	ref2, err := GenerateNormal(tiled2, cfg)
	if err != nil {
		t.Fatal(err)
	}
	differ := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x != 0 && y != 0 && x != w-1 && y != h-1 {
				continue // only inspect the border ring
			}
			for c := 0; c < 3; c++ {
				if math.Abs(float64(n2.At(x, y, c)-ref2.At(x+w, y+h, c))) > 1e-3 {
					differ++
				}
			}
		}
	}
	if differ == 0 {
		t.Fatal("expected clamped (non-wrapped) edges to differ from tiled reference")
	}
}

func TestNormalStrengthScalesGradient(t *testing.T) {
	src := heightFunc(8, 8, func(x, y int) float32 { return float32(x) / 8 })
	weak, _ := GenerateNormal(src, NormalConfig{Strength: 1, Filter: CentralDifference, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true})
	strong, _ := GenerateNormal(src, NormalConfig{Strength: 8, Filter: CentralDifference, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true})
	wx, _, wz := decodeNormal(weak, 4, 4)
	sx, _, sz := decodeNormal(strong, 4, 4)
	if math.Abs(float64(sx)) <= math.Abs(float64(wx)) {
		t.Fatalf("stronger strength should tilt normal more: weak nx=%v strong nx=%v", wx, sx)
	}
	if sz >= wz {
		t.Fatalf("stronger strength should lower nz: weak nz=%v strong nz=%v", wz, sz)
	}
}

func TestNormalRejectsBadSourceChannel(t *testing.T) {
	src := heightFunc(4, 4, func(x, y int) float32 { return 0 })
	_, err := GenerateNormal(src, NormalConfig{SourceChannel: 1})
	if err == nil {
		t.Fatal("expected error for missing source channel")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Kind != ErrChannelInsufficient {
		t.Fatalf("expected ErrChannelInsufficient, got %v", err)
	}
}
