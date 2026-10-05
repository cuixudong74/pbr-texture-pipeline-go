// Command pbrtp demonstrates the in-process texture pipeline: it
// synthesizes source maps, derives a tangent-space normal map from a
// height map, packs data maps into an ORM-style texture, and shows that a
// second run reuses cached artifacts. No external services are involved.
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	tex "github.com/cuixudong74/pbr-texture-pipeline-go/tex"
)

func synthHeight(w, h int) *tex.Image {
	img := tex.NewImage(w, h, 1, tex.Linear, tex.Direction{OriginTopLeft: true})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Periodic pattern so the normal map tiles seamlessly.
			v := math.Sin(2*math.Pi*float64(x)/float64(w)) * math.Cos(2*math.Pi*float64(y)/float64(h))
			img.Set(x, y, 0, float32(0.5+0.5*v))
		}
	}
	return img
}

func synthData(w, h int, base float32) *tex.Image {
	img := tex.NewImage(w, h, 1, tex.Linear, tex.Direction{OriginTopLeft: true})
	for k := range img.Data {
		img.Data[k] = base + float32((k*31)%64)/512
	}
	return img
}

func main() {
	cacheDir := filepath.Join(os.TempDir(), "pbrtp-cache")
	cache, err := tex.NewCache(cacheDir)
	if err != nil {
		log.Fatal(err)
	}
	pipe := tex.NewPipeline(cache, 4)

	const w, h = 128, 128
	height := synthHeight(w, h)
	occlusion := synthData(w, h, 0.8)
	roughness := synthData(w, h, 0.4)
	metallic := synthData(w, h, 0.0)

	normalCfg := tex.NormalConfig{Method: tex.GradientSobel, Strength: 2, YUp: true}
	packCfg := tex.PackConfig{
		Sources: []tex.ChannelSource{
			{ImageIndex: 0, Channel: 0}, // R <- occlusion
			{ImageIndex: 1, Channel: 0}, // G <- roughness
			{ImageIndex: 2, Channel: 0}, // B <- metallic
		},
		OutChannels: 3,
	}
	dataMaps := []*tex.Image{occlusion, roughness, metallic}
	if err := tex.ValidateInputs(dataMaps, []int{1, 1, 1}); err != nil {
		log.Fatal(err)
	}

	jobs := []tex.Job{
		{
			Kind: tex.JobNormal, Name: "normal",
			Inputs: []string{tex.HashImage(height)}, Config: normalCfg.Canonical(),
			Run: func(ctx context.Context) (*tex.Image, error) {
				return tex.HeightToNormal(height, normalCfg)
			},
		},
		{
			Kind: tex.JobPack, Name: "orm",
			Inputs: []string{tex.HashImage(occlusion), tex.HashImage(roughness), tex.HashImage(metallic)},
			Config: packCfg.Canonical(),
			Run: func(ctx context.Context) (*tex.Image, error) {
				return tex.PackChannels(dataMaps, packCfg)
			},
		},
	}

	for run := 1; run <= 2; run++ {
		fmt.Printf("run %d (cache: %s)\n", run, cacheDir)
		for _, r := range pipe.Run(context.Background(), jobs) {
			if r.Err != nil {
				log.Fatalf("job %s: %v", r.Name, r.Err)
			}
			origin := "regenerated"
			if r.FromCache {
				origin = "cache-hit"
			}
			fmt.Printf("  %-8s %-11s %dx%dx%d %s\n", r.Name, origin,
				r.Image.Width, r.Image.Height, r.Image.Channels, r.Image.Space)
		}
	}
}
