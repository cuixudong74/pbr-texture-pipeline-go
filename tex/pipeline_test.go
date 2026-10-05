package tex

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// normalJob builds a normal-map job over src with the given config.
func normalJob(name string, src *Image, cfg NormalConfig) Job {
	return Job{
		Kind:   JobNormal,
		Name:   name,
		Inputs: []string{HashImage(src)},
		Config: cfg.Canonical(),
		Run: func(ctx context.Context) (*Image, error) {
			return HeightToNormal(src, cfg)
		},
	}
}

func testHeight(w, h int) *Image {
	return makeHeight(w, h, func(x, y int) float32 {
		return float32((x*x+y)%16) / 16
	})
}

func TestPipelineIncrementalMatchesFullRebuild(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := NewPipeline(cache, 4)
	srcA := testHeight(32, 32)
	srcB := testHeight(32, 32)
	srcB.Data[0] = 0.9 // different content
	cfg := NormalConfig{Method: GradientSobel, Strength: 1.5, YUp: true}

	jobs := []Job{normalJob("nA", srcA, cfg), normalJob("nB", srcB, cfg)}

	// First run: everything is generated.
	first := p.Run(context.Background(), jobs)
	for _, r := range first {
		if r.Err != nil || r.FromCache {
			t.Fatalf("first run: %+v", r)
		}
	}

	// Second run with one changed input: only that job regenerates.
	srcA2 := srcA.Clone()
	srcA2.Data[5] = 0.42
	jobs2 := []Job{normalJob("nA2", srcA2, cfg), normalJob("nB", srcB, cfg)}
	second := p.Run(context.Background(), jobs2)
	if second[0].Err != nil || second[0].FromCache {
		t.Fatal("changed content must regenerate")
	}
	if second[1].Err != nil || !second[1].FromCache {
		t.Fatal("unchanged content must be reused")
	}

	// Full rebuild with a fresh cache must be byte-identical to the
	// incrementally produced artifacts.
	freshCache, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	full := NewPipeline(freshCache, 1).Run(context.Background(), jobs2)
	for i := range jobs2 {
		if full[i].Err != nil {
			t.Fatal(full[i].Err)
		}
		if !full[i].Image.Equal(second[i].Image) {
			t.Fatalf("job %d: incremental result differs from full rebuild", i)
		}
		if string(EncodeImage(full[i].Image)) != string(EncodeImage(second[i].Image)) {
			t.Fatalf("job %d: incremental bytes differ from full rebuild", i)
		}
	}

	// Config change invalidates: same content, different strength.
	jobs3 := []Job{normalJob("nB", srcB, NormalConfig{Method: GradientSobel, Strength: 3, YUp: true})}
	third := p.Run(context.Background(), jobs3)
	if third[0].FromCache || third[0].Err != nil {
		t.Fatal("config change must invalidate the cached artifact")
	}
}

func TestPipelineReportsReusePerArtifact(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	p := NewPipeline(cache, 2)
	src := testHeight(16, 16)
	cfg := NormalConfig{Method: GradientCentral, Strength: 1, YUp: false}
	jobs := []Job{normalJob("a", src, cfg), normalJob("b", src, cfg)}
	p.Run(context.Background(), jobs)
	res := p.Run(context.Background(), jobs)
	for _, r := range res {
		if !r.FromCache {
			t.Fatalf("job %s should report cache reuse", r.Name)
		}
	}
}

func TestPipelineConcurrencyBound(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	const limit = 2
	p := NewPipeline(cache, limit)
	var cur, maxSeen atomic.Int32
	var jobs []Job
	for i := 0; i < 10; i++ {
		jobs = append(jobs, Job{
			Kind: JobNormal, Name: string(rune('a' + i)),
			Inputs: []string{string(rune('a' + i))},
			Config: "cfg",
			Run: func(ctx context.Context) (*Image, error) {
				n := cur.Add(1)
				for {
					m := maxSeen.Load()
					if n <= m || maxSeen.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				cur.Add(-1)
				return NewImage(2, 2, 1, Linear, Direction{}), nil
			},
		})
	}
	res := p.Run(context.Background(), jobs)
	for _, r := range res {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
	}
	if got := maxSeen.Load(); got > limit {
		t.Fatalf("concurrency %d exceeded limit %d", got, limit)
	}
}

func TestPipelineCancellationLeavesNoPartialState(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	p := NewPipeline(cache, 4)
	src := testHeight(64, 64)
	cfg := NormalConfig{Method: GradientSobel, Strength: 1, YUp: true}

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	slowJob := Job{
		Kind: JobNormal, Name: "slow",
		Inputs: []string{HashImage(src)}, Config: cfg.Canonical(),
		Run: func(ctx context.Context) (*Image, error) {
			close(started)
			<-ctx.Done() // simulate a slow task that honors cancellation
			return nil, ctx.Err()
		},
	}
	go func() {
		<-started
		cancel()
	}()
	res := p.Run(ctx, []Job{slowJob})
	if res[0].Err == nil {
		t.Fatal("cancelled job must report an error")
	}
	// The cancelled job must not have polluted the cache: rerunning with a
	// fresh context regenerates (no cache hit) and succeeds.
	res2 := p.Run(context.Background(), []Job{normalJob("slow", src, cfg)})
	if res2[0].Err != nil {
		t.Fatal(res2[0].Err)
	}
	if res2[0].FromCache {
		t.Fatal("cancelled job left a cache entry behind")
	}
}

func TestPipelineTimeout(t *testing.T) {
	cache, _ := NewCache(t.TempDir())
	p := NewPipeline(cache, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	job := Job{
		Kind: JobPack, Name: "t",
		Inputs: []string{"x"}, Config: "c",
		Run: func(ctx context.Context) (*Image, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Hour):
				return NewImage(1, 1, 1, Linear, Direction{}), nil
			}
		},
	}
	res := p.Run(ctx, []Job{job})
	if res[0].Err == nil {
		t.Fatal("timed-out job must report an error")
	}
}
