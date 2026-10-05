package tex

import "fmt"

// ErrKind classifies validation failures so callers can distinguish causes.
type ErrKind int

const (
	// ErrSizeMismatch: inputs do not share the same dimensions.
	ErrSizeMismatch ErrKind = iota
	// ErrChannelMismatch: an input has fewer channels than required.
	ErrChannelMismatch
	// ErrDirectionConflict: inputs carry conflicting orientation metadata.
	ErrDirectionConflict
	// ErrInvalidConfig: a configuration value is unusable.
	ErrInvalidConfig
)

func (k ErrKind) String() string {
	switch k {
	case ErrSizeMismatch:
		return "size-mismatch"
	case ErrChannelMismatch:
		return "channel-mismatch"
	case ErrDirectionConflict:
		return "direction-conflict"
	case ErrInvalidConfig:
		return "invalid-config"
	}
	return "unknown"
}

// ValidationError carries a kind plus which inputs are involved, so a
// failure can be located and told apart from other failure classes.
type ValidationError struct {
	Kind    ErrKind
	Inputs  []int // indices of offending inputs
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func newValidationError(kind ErrKind, inputs []int, format string, args ...any) *ValidationError {
	return &ValidationError{Kind: kind, Inputs: inputs, Message: fmt.Sprintf(format, args...)}
}

// ValidateInputs checks a batch of images before any processing starts:
//   - at least one image, none nil;
//   - all images share the same dimensions (ErrSizeMismatch);
//   - image i has at least requiredChannels[i] channels (ErrChannelMismatch);
//   - all images agree on direction metadata (ErrDirectionConflict).
//
// requiredChannels may be nil or shorter than imgs (missing entries mean
// "at least 1 channel"). Any failure rejects the whole batch; nothing is
// produced from a partially valid set.
func ValidateInputs(imgs []*Image, requiredChannels []int) error {
	if len(imgs) == 0 {
		return newValidationError(ErrInvalidConfig, nil, "validate: no input images")
	}
	for i, img := range imgs {
		if img == nil {
			return newValidationError(ErrInvalidConfig, []int{i}, "validate: input %d is nil", i)
		}
	}
	w, h := imgs[0].Width, imgs[0].Height
	for i := 1; i < len(imgs); i++ {
		if imgs[i].Width != w || imgs[i].Height != h {
			return newValidationError(ErrSizeMismatch, []int{0, i},
				"validate: input %d is %dx%d, input 0 is %dx%d",
				i, imgs[i].Width, imgs[i].Height, w, h)
		}
	}
	for i, img := range imgs {
		need := 1
		if i < len(requiredChannels) {
			need = requiredChannels[i]
		}
		if img.Channels < need {
			return newValidationError(ErrChannelMismatch, []int{i},
				"validate: input %d has %d channels, need at least %d", i, img.Channels, need)
		}
	}
	dir := imgs[0].Dir
	for i := 1; i < len(imgs); i++ {
		if imgs[i].Dir != dir {
			return newValidationError(ErrDirectionConflict, []int{0, i},
				"validate: input %d direction %+v conflicts with input 0 direction %+v",
				i, imgs[i].Dir, dir)
		}
	}
	return nil
}
