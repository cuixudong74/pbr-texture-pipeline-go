package pipeline

import (
	"context"
	"fmt"
	"sync"
)

// JobKind selects the processing operation of a Job.
type JobKind int

const (
	JobNormalMap JobKind = iota
	JobChannelPack
)

// Job describes one artifact to produce.
type Job struct {
	Name string
	Kind JobKind
	// Inputs are the source textures for this job.
	Inputs []*Texture
	// Normal is used when Kind == JobNormalMap.
	Normal NormalConfig
	// Pack is used when Kind == JobChannelPack.
	Pack PackConfig
}

// Artifact is one produced (or reused) output.
type Artifact struct {
	Name    string
	Texture *Texture
	// Reused is true when the result came from the cache unchanged.
	Reused bool
}

// Result is the outcome of a Run.
type Result struct {
	Artifacts []Artifact
}

// Pipeline executes jobs with bounded concurrency and memory, caching results.
type Pipeline struct {
	Cache *Cache
	// MaxConcurrency bounds simultaneously executing jobs (<=0 means 4).
	MaxConcurrency int
	// MaxMemoryBytes bounds total estimated working memory (<=0 means 1 GiB).
	MaxMemoryBytes int64
}

func (p *Pipeline) concurrency() int {
	if p.MaxConcurrency <= 0 {
		return 4
	}
	return p.MaxConcurrency
}

func (p *Pipeline) memoryBudget() int64 {
	if p.MaxMemoryBytes <= 0 {
		return 1 << 30
	}
	return p.MaxMemoryBytes
}

// estimateMemory approximates the working set of a job in bytes.
func estimateMemory(j Job) int64 {
	var n int64
	for _, t := range j.Inputs {
		if t != nil {
			n += int64(len(t.Data)) * 4
		}
	}
	if len(j.Inputs) > 0 && j.Inputs[0] != nil {
		w, h := int64(j.Inputs[0].Width), int64(j.Inputs[0].Height)
		switch j.Kind {
		case JobNormalMap:
			n += w * h * 3 * 4
		case JobChannelPack:
			n += w * h * int64(len(j.Pack.Sources)) * 4
		}
	}
	if n <= 0 {
		n = 1
	}
	return n
}

// validateJob rejects inconsistent inputs before any processing starts.
func validateJob(j Job) error {
	switch j.Kind {
	case JobNormalMap:
		if len(j.Inputs) != 1 {
			return &ValidationError{Kind: ErrInvalidConfig,
				Message: fmt.Sprintf("normal job %q wants exactly 1 input, got %d", j.Name, len(j.Inputs))}
		}
		src := j.Inputs[0]
		if src == nil {
			return &ValidationError{Kind: ErrInvalidConfig, Message: "normal job " + j.Name + ": nil input"}
		}
		if j.Normal.SourceChannel < 0 || j.Normal.SourceChannel >= src.Channels {
			return &ValidationError{Kind: ErrChannelInsufficient,
				Message: fmt.Sprintf("normal job %q: source channel %d, input has %d channels",
					j.Name, j.Normal.SourceChannel, src.Channels),
				Inputs: []int{0}}
		}
	case JobChannelPack:
		need := make([]int, len(j.Inputs))
		for i, s := range j.Pack.Sources {
			if s.Input == -1 {
				continue
			}
			if s.Input < 0 || s.Input >= len(j.Inputs) {
				return &ValidationError{Kind: ErrInvalidConfig,
					Message: fmt.Sprintf("pack job %q: output channel %d references input %d, have %d",
						j.Name, i, s.Input, len(j.Inputs))}
			}
			if s.Channel+1 > need[s.Input] {
				need[s.Input] = s.Channel + 1
			}
		}
		if err := ValidateInputs(j.Inputs, need); err != nil {
			return err
		}
	default:
		return &ValidationError{Kind: ErrInvalidConfig,
			Message: fmt.Sprintf("job %q: unknown kind %d", j.Name, j.Kind)}
	}
	return nil
}

// jobKey computes the cache key for a job from its config and input content.
func jobKey(j Job) [32]byte {
	hashes := make([][32]byte, len(j.Inputs))
	for i, t := range j.Inputs {
		hashes[i] = t.ContentHash()
	}
	switch j.Kind {
	case JobNormalMap:
		return Key("normal", j.Normal, hashes...)
	case JobChannelPack:
		return Key("pack", j.Pack, hashes...)
	}
	return [32]byte{}
}

// process executes a single job (validation already done).
func process(j Job) (*Texture, error) {
	switch j.Kind {
	case JobNormalMap:
		return GenerateNormal(j.Inputs[0], j.Normal)
	case JobChannelPack:
		return Pack(j.Inputs, j.Pack)
	}
	return nil, fmt.Errorf("unknown job kind %d", j.Kind)
}

// Run executes all jobs, honoring ctx cancellation. Artifacts whose inputs
// and config are unchanged are reused from the cache; the rest are
// regenerated and cached. Incremental output is byte-identical to a full
// rebuild because every artifact is a pure function of its cache key.
func (p *Pipeline) Run(ctx context.Context, jobs []Job) (*Result, error) {
	// Fail fast: reject any inconsistent input set before processing begins.
	for _, j := range jobs {
		if err := validateJob(j); err != nil {
			return nil, err
		}
	}

	res := &Result{Artifacts: make([]Artifact, len(jobs))}
	sem := make(chan struct{}, p.concurrency())
	// Memory is tracked in 64 KiB units so acquire/release loops stay cheap
	// even for very large images.
	const memUnit = 64 * 1024
	memCap := (p.memoryBudget() + memUnit - 1) / memUnit
	memTokens := make(chan struct{}, memCap)

	// memUnits converts a byte estimate to tokens. A job larger than the
	// whole budget is clamped to the full capacity: it then runs alone
	// instead of deadlocking.
	memUnits := func(bytes int64) int64 {
		n := (bytes + memUnit - 1) / memUnit
		if n > memCap {
			n = memCap
		}
		if n < 1 {
			n = 1
		}
		return n
	}

	acquireMem := func(bytes int64) error {
		n := memUnits(bytes)
		for i := int64(0); i < n; i++ {
			select {
			case memTokens <- struct{}{}:
			case <-ctx.Done():
				// Release what we took so far.
				for k := int64(0); k < i; k++ {
					<-memTokens
				}
				return ctx.Err()
			}
		}
		return nil
	}
	releaseMem := func(bytes int64) {
		n := memUnits(bytes)
		for i := int64(0); i < n; i++ {
			<-memTokens
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(jobs))
	for i, j := range jobs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := jobKey(j)
		if p.Cache != nil {
			if payload := p.Cache.Get(key); payload != nil {
				tex, err := DecodeTexture(payload)
				if err == nil {
					res.Artifacts[i] = Artifact{Name: j.Name, Texture: tex, Reused: true}
					continue
				}
				// Corrupt payload: treat as miss and regenerate.
			}
		}
		mem := estimateMemory(j)
		if err := acquireMem(mem); err != nil {
			wg.Wait()
			return nil, err
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			releaseMem(mem)
			wg.Wait()
			return nil, ctx.Err()
		}
		wg.Add(1)
		go func(i int, j Job, key [32]byte, mem int64) {
			defer wg.Done()
			defer func() { <-sem }()
			defer releaseMem(mem)
			tex, err := process(j)
			if err != nil {
				select {
				case errCh <- fmt.Errorf("job %q: %w", j.Name, err):
				default:
				}
				return
			}
			if p.Cache != nil {
				if payload, err := EncodeTexture(tex); err == nil {
					// Atomic put: a cancelled or crashed run never leaves a
					// partial artifact visible to later runs.
					_ = p.Cache.Put(key, payload)
				}
			}
			res.Artifacts[i] = Artifact{Name: j.Name, Texture: tex, Reused: false}
		}(i, j, key, mem)
	}
	wg.Wait()
	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return res, nil
}
