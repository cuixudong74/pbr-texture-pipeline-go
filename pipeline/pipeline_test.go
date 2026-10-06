package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleJobs() []Job {
	height := heightFunc(32, 32, func(x, y int) float32 {
		return float32((x*x+y*y)%64) / 64
	})
	rough := grayTex(32, 32, func(x, y int) float32 { return float32(x) / 32 })
	metal := grayTex(32, 32, func(x, y int) float32 { return float32(y) / 32 })
	ao := grayTex(32, 32, func(x, y int) float32 { return 0.8 })
	return []Job{
		{Name: "normal", Kind: JobNormalMap, Inputs: []*Texture{height},
			Normal: NormalConfig{Strength: 2, Filter: Sobel, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true}},
		{Name: "orm", Kind: JobChannelPack, Inputs: []*Texture{ao, rough, metal},
			Pack: PackConfig{Sources: []ChannelSource{{Input: 0}, {Input: 1}, {Input: 2}}}},
	}
}

func artifactBytes(t *testing.T, r *Result) map[string][]byte {
	t.Helper()
	m := map[string][]byte{}
	for _, a := range r.Artifacts {
		b, err := EncodeTexture(a.Texture)
		if err != nil {
			t.Fatal(err)
		}
		m[a.Name] = b
	}
	return m
}

func TestIncrementalMatchesFullRebuild(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{Cache: cache, MaxConcurrency: 4}
	jobs := sampleJobs()

	// First run: everything regenerated.
	r1, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r1.Artifacts {
		if a.Reused {
			t.Fatalf("first run must regenerate, %q was reused", a.Name)
		}
	}

	// Second run, unchanged: everything reused, byte-identical.
	r2, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r2.Artifacts {
		if !a.Reused {
			t.Fatalf("unchanged job %q must be reused", a.Name)
		}
	}
	b1, b2 := artifactBytes(t, r1), artifactBytes(t, r2)
	for name := range b1 {
		if string(b1[name]) != string(b2[name]) {
			t.Fatalf("artifact %q differs between full and incremental run", name)
		}
	}

	// Change one input: only that job regenerates; the other is reused and
	// still byte-identical.
	jobs2 := sampleJobs()
	jobs2[0].Inputs[0].Set(3, 3, 0, 0.01) // touch height map
	r3, err := p.Run(context.Background(), jobs2)
	if err != nil {
		t.Fatal(err)
	}
	reused := map[string]bool{}
	for _, a := range r3.Artifacts {
		reused[a.Name] = a.Reused
	}
	if reused["normal"] {
		t.Fatal("changed normal job must regenerate")
	}
	if !reused["orm"] {
		t.Fatal("unchanged orm job must be reused")
	}
	b3 := artifactBytes(t, r3)
	if string(b3["orm"]) != string(b1["orm"]) {
		t.Fatal("reused orm artifact bytes differ from full run")
	}
	if string(b3["normal"]) == string(b1["normal"]) {
		t.Fatal("regenerated normal artifact should differ after input change")
	}
}

func TestCacheInvalidatesOnConfigChange(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	p := &Pipeline{Cache: cache}
	jobs := sampleJobs()
	if _, err := p.Run(context.Background(), jobs); err != nil {
		t.Fatal(err)
	}
	jobs[0].Normal.Strength = 5 // config change must invalidate
	r, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r.Artifacts {
		if a.Name == "normal" && a.Reused {
			t.Fatal("config change must invalidate cached normal map")
		}
		if a.Name == "orm" && !a.Reused {
			t.Fatal("unrelated job must stay cached")
		}
	}
}

func TestCacheRejectsCorruptEntries(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	p := &Pipeline{Cache: cache}
	jobs := sampleJobs()
	r1, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt every cache file.
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("expected cache entries on disk")
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		b, _ := os.ReadFile(path)
		b[len(b)/2] ^= 0xFF
		os.WriteFile(path, b, 0o644)
	}
	// Corrupt entries must not be served; results regenerate and still match.
	r2, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r2.Artifacts {
		if a.Reused {
			t.Fatalf("corrupt cache entry served for %q", a.Name)
		}
	}
	b1, b2 := artifactBytes(t, r1), artifactBytes(t, r2)
	for name := range b1 {
		if string(b1[name]) != string(b2[name]) {
			t.Fatalf("regenerated %q differs after cache corruption", name)
		}
	}
	// Truncated (incompletely written) entry must also be rejected.
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		b, _ := os.ReadFile(path)
		os.WriteFile(path, b[:len(b)/2], 0o644)
	}
	r3, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range r3.Artifacts {
		if a.Reused {
			t.Fatalf("truncated cache entry served for %q", a.Name)
		}
	}
}

func TestRunRejectsInvalidInputsBeforeProcessing(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	p := &Pipeline{Cache: cache}
	jobs := sampleJobs()
	// Break the second job: mismatched size.
	jobs[1].Inputs[1] = grayTex(16, 32, func(x, y int) float32 { return 0 })
	_, err := p.Run(context.Background(), jobs)
	if err == nil {
		t.Fatal("expected validation error")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Kind != ErrSizeMismatch {
		t.Fatalf("want size mismatch, got %v", err)
	}
	// Nothing may be cached: no partial output from the aborted batch.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("aborted run left %d cache entries", len(entries))
	}
}

func TestRunCancellation(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	p := &Pipeline{Cache: cache, MaxConcurrency: 1}
	// Large jobs so cancellation lands mid-batch.
	big := heightFunc(1024, 1024, func(x, y int) float32 { return float32((x*y)%97) / 97 })
	jobs := make([]Job, 8)
	for i := range jobs {
		jobs[i] = Job{Name: "n", Kind: JobNormalMap, Inputs: []*Texture{big},
			Normal: NormalConfig{Strength: 1, Filter: Sobel, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true}}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, err := p.Run(ctx, jobs)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	// A later run with a fresh context must work and not be poisoned by the
	// cancelled attempt.
	r, err := p.Run(context.Background(), jobs[:1])
	if err != nil {
		t.Fatalf("run after cancellation failed: %v", err)
	}
	if len(r.Artifacts) != 1 || r.Artifacts[0].Texture == nil {
		t.Fatal("run after cancellation produced no artifact")
	}
}

func TestRunRespectsMemoryBound(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	// Budget smaller than one job's estimate forces strictly serial progress
	// but must still complete (a single oversized job may proceed alone).
	p := &Pipeline{Cache: cache, MaxConcurrency: 8, MaxMemoryBytes: 1 << 20}
	jobs := sampleJobs()
	r, err := p.Run(context.Background(), jobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != len(jobs) {
		t.Fatalf("got %d artifacts, want %d", len(r.Artifacts), len(jobs))
	}
}

func TestRunOversizedJobDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	cache, _ := NewCache(dir)
	// Budget far below a single job's estimate: the job must still complete.
	p := &Pipeline{Cache: cache, MaxConcurrency: 2, MaxMemoryBytes: 64 * 1024}
	jobs := sampleJobs() // each job estimates well above 64 KiB? ensure it
	big := heightFunc(256, 256, func(x, y int) float32 { return float32(x+y) / 512 })
	jobs = append(jobs, Job{Name: "big", Kind: JobNormalMap, Inputs: []*Texture{big},
		Normal: NormalConfig{Strength: 1, Filter: Sobel, Convention: OpenGLNormals, WrapHorizontal: true, WrapVertical: true}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := p.Run(ctx, jobs)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Artifacts) != len(jobs) {
		t.Fatalf("got %d artifacts, want %d", len(r.Artifacts), len(jobs))
	}
}
