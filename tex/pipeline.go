package tex

import (
	"context"
	"sync"
)

// JobKind identifies a derivation step.
type JobKind int

const (
	JobNormal JobKind = iota
	JobPack
	JobColorConvert
)

func (k JobKind) String() string {
	switch k {
	case JobNormal:
		return "normal"
	case JobPack:
		return "pack"
	case JobColorConvert:
		return "color-convert"
	}
	return "unknown"
}

// Job describes one derivation step. The cache key is derived from Kind,
// Inputs (content hashes of sources) and Config (canonical config
// encoding), so a job re-runs exactly when its content or config changed.
type Job struct {
	Kind   JobKind
	Name   string
	Inputs []string
	Config string

	// Run performs the actual work on a cache miss. It must honor ctx:
	// when ctx is cancelled it should abort and return ctx.Err().
	Run func(ctx context.Context) (*Image, error)
}

// Result reports the outcome of one job.
type Result struct {
	Name      string
	Image     *Image
	FromCache bool // true when the artifact was reused, false when regenerated
	Err       error
}

// Pipeline executes jobs with bounded concurrency and content-keyed
// caching. The concurrency limit bounds both CPU and peak memory: at most
// that many jobs (and their images) are in flight at once.
type Pipeline struct {
	cache       *Cache
	concurrency int
}

// NewPipeline creates a pipeline. cache may be nil (no reuse across runs).
// concurrency <= 0 means a default of 4.
func NewPipeline(cache *Cache, concurrency int) *Pipeline {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Pipeline{cache: cache, concurrency: concurrency}
}

// Run executes all jobs, respecting ctx cancellation, and returns one
// Result per job in input order. A cancelled or failed job never writes
// to the cache, so no partial artifact can pollute later runs.
func (p *Pipeline) Run(ctx context.Context, jobs []Job) []Result {
	results := make([]Result, len(jobs))
	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job Job) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = Result{Name: job.Name, Err: ctx.Err()}
				return
			}
			results[i] = p.runOne(ctx, job)
		}(i, job)
	}
	wg.Wait()
	return results
}

func (p *Pipeline) runOne(ctx context.Context, job Job) Result {
	res := Result{Name: job.Name}
	if err := ctx.Err(); err != nil {
		res.Err = err
		return res
	}
	key := Key(job.Kind.String(), job.Inputs, job.Config)
	if p.cache != nil {
		if entry := p.cache.Get(key); entry != nil {
			img, err := DecodeImage(entry.Data)
			if err == nil {
				res.Image = img
				res.FromCache = true
				return res
			}
			// Undecodable payload: treat as a miss and regenerate.
		}
	}
	img, err := job.Run(ctx)
	if err != nil {
		res.Err = err
		return res
	}
	// Do not cache results produced while the context was cancelled; the
	// work may have been cut short.
	if ctx.Err() != nil {
		res.Err = ctx.Err()
		return res
	}
	if p.cache != nil {
		if err := p.cache.Put(key, EncodeImage(img)); err != nil {
			res.Err = err
			return res
		}
	}
	res.Image = img
	return res
}
