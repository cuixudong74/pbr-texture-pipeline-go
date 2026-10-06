// Command texturegen demonstrates the pipeline end to end: it synthesizes
// sample source maps in-process, derives a normal map and a packed ORM map,
// caches results under -cache, and writes PNG previews to -out. Run it twice
// to see incremental reuse reported per artifact.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	pipe "github.com/cuixudong74/pbr-texture-pipeline-go/pipeline"
)

func clamp8(v float32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return uint8(v*255 + 0.5)
}

func savePNG(t *pipe.Texture, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, t.Width, t.Height))
	for y := 0; y < t.Height; y++ {
		for x := 0; x < t.Width; x++ {
			var r, g, b, a uint8 = 0, 0, 0, 255
			switch t.Channels {
			case 1:
				r, g, b = clamp8(t.At(x, y, 0)), clamp8(t.At(x, y, 0)), clamp8(t.At(x, y, 0))
			case 3:
				r, g, b = clamp8(t.At(x, y, 0)), clamp8(t.At(x, y, 1)), clamp8(t.At(x, y, 2))
			case 4:
				r, g, b, a = clamp8(t.At(x, y, 0)), clamp8(t.At(x, y, 1)), clamp8(t.At(x, y, 2)), clamp8(t.At(x, y, 3))
			}
			img.Set(x, y, color.RGBA{r, g, b, a})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	outDir := flag.String("out", "out", "directory for PNG previews")
	cacheDir := flag.String("cache", "cache", "cache directory")
	flag.Parse()

	cache, err := pipe.NewCache(*cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	const size = 256
	// Sample height map: a few tileable sine octaves.
	height := pipe.NewTexture(size, size, 1, pipe.Linear, pipe.OrientationYDown)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)/size, float64(y)/size
			v := 0.5 + 0.25*math.Sin(2*math.Pi*4*fx)*math.Cos(2*math.Pi*4*fy) +
				0.125*math.Sin(2*math.Pi*8*fx+1.3)*math.Cos(2*math.Pi*8*fy)
			height.Set(x, y, 0, float32(v))
		}
	}
	rough := pipe.NewTexture(size, size, 1, pipe.Linear, pipe.OrientationYDown)
	metal := pipe.NewTexture(size, size, 1, pipe.Linear, pipe.OrientationYDown)
	ao := pipe.NewTexture(size, size, 1, pipe.Linear, pipe.OrientationYDown)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			rough.Set(x, y, 0, float32(x)/size)
			metal.Set(x, y, 0, float32(y)/size)
			ao.Set(x, y, 0, 0.9)
		}
	}

	jobs := []pipe.Job{
		{Name: "normal", Kind: pipe.JobNormalMap, Inputs: []*pipe.Texture{height},
			Normal: pipe.NormalConfig{Strength: 3, Filter: pipe.Sobel, Convention: pipe.OpenGLNormals,
				WrapHorizontal: true, WrapVertical: true}},
		{Name: "orm", Kind: pipe.JobChannelPack, Inputs: []*pipe.Texture{ao, rough, metal},
			Pack: pipe.PackConfig{Sources: []pipe.ChannelSource{{Input: 0}, {Input: 1}, {Input: 2}}}},
	}

	p := &pipe.Pipeline{Cache: cache, MaxConcurrency: 4}
	res, err := p.Run(context.Background(), jobs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, a := range res.Artifacts {
		state := "regenerated"
		if a.Reused {
			state = "reused (cache hit)"
		}
		fmt.Printf("%-8s %s\n", a.Name, state)
		if err := savePNG(a.Texture, filepath.Join(*outDir, a.Name+".png")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
