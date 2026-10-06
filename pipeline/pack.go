package pipeline

import "fmt"

// ChannelSource maps one output channel to a channel of an input texture.
// Input == -1 means "use the configured default value for this channel".
type ChannelSource struct {
	Input   int // index into the inputs slice, or -1 for default
	Channel int // channel within that input
	Invert  bool
}

// PackConfig uniquely determines how inputs are merged into the output:
// output channel i is taken from Sources[i] (or Defaults[i] when the source
// input is -1). The mapping is fully explicit, so the merge order and channel
// assignment are fixed by configuration alone.
type PackConfig struct {
	// Sources has one entry per output channel, in order R,G,B,A.
	Sources []ChannelSource
	// Defaults holds the fill value per output channel used when the
	// corresponding source has Input == -1. Missing entries default to 0.
	Defaults []float32
}

// Pack merges data textures into one texture. Validation happens before any
// output is produced, and each output channel is written exactly once from
// exactly one source, so the result is bit-identical regardless of
// evaluation order or concurrency. The output is always Linear (data maps
// must not be gamma-transformed).
func Pack(inputs []*Texture, cfg PackConfig) (*Texture, error) {
	if len(cfg.Sources) == 0 {
		return nil, &ValidationError{Kind: ErrInvalidConfig, Message: "pack config has no sources"}
	}
	if len(cfg.Sources) > 4 {
		return nil, &ValidationError{
			Kind:    ErrInvalidConfig,
			Message: fmt.Sprintf("pack config has %d output channels, max 4", len(cfg.Sources)),
		}
	}
	// Validate sizes/orientation across all referenced inputs up front.
	need := make([]int, len(inputs))
	for i, s := range cfg.Sources {
		if s.Input == -1 {
			continue
		}
		if s.Input < 0 || s.Input >= len(inputs) {
			return nil, &ValidationError{
				Kind:    ErrInvalidConfig,
				Message: fmt.Sprintf("output channel %d references input %d, have %d inputs", i, s.Input, len(inputs)),
			}
		}
		if s.Channel < 0 {
			return nil, &ValidationError{
				Kind:    ErrInvalidConfig,
				Message: fmt.Sprintf("output channel %d references negative source channel %d", i, s.Channel),
			}
		}
		if s.Channel+1 > need[s.Input] {
			need[s.Input] = s.Channel + 1
		}
	}
	if err := ValidateInputs(inputs, need); err != nil {
		return nil, err
	}

	w, h := inputs[0].Width, inputs[0].Height
	out := NewTexture(w, h, len(cfg.Sources), Linear, inputs[0].Orientation)
	// Decode any color-encoded inputs to linear first; data maps pass through.
	lin := make([]*Texture, len(inputs))
	for i, t := range inputs {
		lin[i] = DecodeToLinear(t)
	}
	for i, s := range cfg.Sources {
		def := float32(0)
		if i < len(cfg.Defaults) {
			def = cfg.Defaults[i]
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := def
				if s.Input != -1 {
					v = lin[s.Input].At(x, y, s.Channel)
					if s.Invert {
						v = 1 - v
					}
				}
				out.Set(x, y, i, v)
			}
		}
	}
	return out, nil
}
