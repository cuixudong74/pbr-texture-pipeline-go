package pipeline

import (
	"errors"
	"testing"
)

func TestValidateInputsDistinguishesFailures(t *testing.T) {
	base := grayTex(4, 4, func(x, y int) float32 { return 0 })

	wrongSize := grayTex(5, 4, func(x, y int) float32 { return 0 })
	err := ValidateInputs([]*Texture{base, wrongSize}, nil)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Kind != ErrSizeMismatch {
		t.Fatalf("want ErrSizeMismatch, got %v", err)
	}
	if len(ve.Inputs) == 0 || ve.Inputs[len(ve.Inputs)-1] != 1 {
		t.Fatalf("error must locate offending input, got %v", ve.Inputs)
	}

	flipped := grayTex(4, 4, func(x, y int) float32 { return 0 })
	flipped.Orientation = OrientationYUp
	err = ValidateInputs([]*Texture{base, flipped}, nil)
	if !errors.As(err, &ve) || ve.Kind != ErrOrientationConflict {
		t.Fatalf("want ErrOrientationConflict, got %v", err)
	}

	err = ValidateInputs([]*Texture{base}, []int{2})
	if !errors.As(err, &ve) || ve.Kind != ErrChannelInsufficient {
		t.Fatalf("want ErrChannelInsufficient, got %v", err)
	}

	if err := ValidateInputs([]*Texture{base, base}, []int{1, 1}); err != nil {
		t.Fatalf("valid inputs rejected: %v", err)
	}
}
