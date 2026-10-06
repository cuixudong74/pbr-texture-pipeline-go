package pipeline

import "fmt"

// ErrorKind classifies validation failures so callers can distinguish causes.
type ErrorKind int

const (
	ErrSizeMismatch ErrorKind = iota
	ErrChannelInsufficient
	ErrOrientationConflict
	ErrColorSpaceConflict
	ErrInvalidConfig
)

func (k ErrorKind) String() string {
	switch k {
	case ErrSizeMismatch:
		return "size mismatch"
	case ErrChannelInsufficient:
		return "insufficient channels"
	case ErrOrientationConflict:
		return "orientation conflict"
	case ErrColorSpaceConflict:
		return "color space conflict"
	case ErrInvalidConfig:
		return "invalid config"
	}
	return "unknown"
}

// ValidationError describes one rejected input set with a locatable cause.
type ValidationError struct {
	Kind    ErrorKind
	Message string
	// Inputs lists the offending input indices when applicable.
	Inputs []int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (inputs %v)", e.Kind, e.Message, e.Inputs)
}

// ValidateInputs checks that all inputs share size and orientation, and that
// each input provides at least requiredChannels[i] channels (0 = no minimum,
// missing entries are treated as 0). It returns the first violation found;
// processing must not start when this reports an error.
func ValidateInputs(inputs []*Texture, requiredChannels []int) error {
	if len(inputs) == 0 {
		return &ValidationError{Kind: ErrInvalidConfig, Message: "no inputs provided"}
	}
	w, h := inputs[0].Width, inputs[0].Height
	o := inputs[0].Orientation
	for i, t := range inputs {
		if t == nil {
			return &ValidationError{
				Kind: ErrInvalidConfig, Message: "nil input texture", Inputs: []int{i},
			}
		}
		if t.Width != w || t.Height != h {
			return &ValidationError{
				Kind: ErrSizeMismatch,
				Message: fmt.Sprintf("input %d is %dx%d, expected %dx%d (same as input 0)",
					i, t.Width, t.Height, w, h),
				Inputs: []int{0, i},
			}
		}
		if t.Orientation != o {
			return &ValidationError{
				Kind: ErrOrientationConflict,
				Message: fmt.Sprintf("input %d has orientation %d, input 0 has %d",
					i, t.Orientation, o),
				Inputs: []int{0, i},
			}
		}
		need := 0
		if i < len(requiredChannels) {
			need = requiredChannels[i]
		}
		if t.Channels < need {
			return &ValidationError{
				Kind: ErrChannelInsufficient,
				Message: fmt.Sprintf("input %d has %d channels, need at least %d",
					i, t.Channels, need),
				Inputs: []int{i},
			}
		}
	}
	return nil
}
