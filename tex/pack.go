package tex

// ChannelSource names one input image and the channel to read from it.
type ChannelSource struct {
	ImageIndex int
	Channel    int
}

// PackConfig uniquely determines how sources map to output channels.
// Output channel i takes Sources[i]; output channels at or beyond
// len(Sources) are filled with FillValue. The mapping is fully determined
// by the config: same inputs + same config always give the same pixels,
// regardless of internal scheduling.
type PackConfig struct {
	Sources     []ChannelSource
	OutChannels int
	FillValue   float32
}

// PackChannels merges channels from multiple images into one output image.
// Every output (pixel, channel) is written exactly once, from exactly one
// source, so the result is deterministic under any concurrency or order.
// Packed maps are data maps: the output is always Linear.
func PackChannels(imgs []*Image, cfg PackConfig) (*Image, error) {
	if len(imgs) == 0 {
		return nil, newValidationError(ErrInvalidConfig, nil, "pack: no input images")
	}
	if cfg.OutChannels <= 0 {
		return nil, newValidationError(ErrInvalidConfig, nil,
			"pack: OutChannels must be > 0, got %d", cfg.OutChannels)
	}
	if len(cfg.Sources) > cfg.OutChannels {
		return nil, newValidationError(ErrInvalidConfig, nil,
			"pack: %d sources for only %d output channels", len(cfg.Sources), cfg.OutChannels)
	}
	w, h := imgs[0].Width, imgs[0].Height
	for si, s := range cfg.Sources {
		if s.ImageIndex < 0 || s.ImageIndex >= len(imgs) {
			return nil, newValidationError(ErrInvalidConfig, []int{si},
				"pack: source %d references image %d, have %d", si, s.ImageIndex, len(imgs))
		}
		img := imgs[s.ImageIndex]
		if img == nil {
			return nil, newValidationError(ErrInvalidConfig, []int{s.ImageIndex},
				"pack: input image %d is nil", s.ImageIndex)
		}
		if img.Width != w || img.Height != h {
			return nil, newValidationError(ErrSizeMismatch, []int{0, s.ImageIndex},
				"pack: image %d is %dx%d, want %dx%d", s.ImageIndex, img.Width, img.Height, w, h)
		}
		if s.Channel < 0 || s.Channel >= img.Channels {
			return nil, newValidationError(ErrChannelMismatch, []int{s.ImageIndex},
				"pack: source %d wants channel %d of image %d which has %d channels",
				si, s.Channel, s.ImageIndex, img.Channels)
		}
	}
	out := NewImage(w, h, cfg.OutChannels, Linear, imgs[0].Dir)
	// Each output channel is an independent copy; writes never overlap, so
	// the result cannot depend on the order channels are processed in.
	for oc := 0; oc < cfg.OutChannels; oc++ {
		if oc >= len(cfg.Sources) {
			for k := 0; k < w*h; k++ {
				out.Data[k*cfg.OutChannels+oc] = cfg.FillValue
			}
			continue
		}
		s := cfg.Sources[oc]
		src := imgs[s.ImageIndex]
		for k := 0; k < w*h; k++ {
			out.Data[k*cfg.OutChannels+oc] = src.Data[k*src.Channels+s.Channel]
		}
	}
	return out, nil
}

// Canonical returns a stable string encoding of the config, suitable for
// cache keys: the full ordered channel mapping is captured.
func (c PackConfig) Canonical() string {
	s := "out=" + itoa(c.OutChannels) + ";fill=" + ftoa(c.FillValue) + ";map="
	for i, src := range c.Sources {
		if i > 0 {
			s += ","
		}
		s += itoa(src.ImageIndex) + ":" + itoa(src.Channel)
	}
	return s
}
