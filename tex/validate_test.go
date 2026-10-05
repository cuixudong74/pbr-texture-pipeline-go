package tex

import "testing"

func TestValidateInputsOK(t *testing.T) {
	a := NewImage(8, 8, 1, Linear, Direction{OriginTopLeft: true})
	b := NewImage(8, 8, 3, Linear, Direction{OriginTopLeft: true})
	if err := ValidateInputs([]*Image{a, b}, []int{1, 3}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSizeMismatch(t *testing.T) {
	a := NewImage(8, 8, 1, Linear, Direction{})
	b := NewImage(8, 4, 1, Linear, Direction{})
	err := ValidateInputs([]*Image{a, b}, nil)
	ve, ok := err.(*ValidationError)
	if !ok || ve.Kind != ErrSizeMismatch {
		t.Fatalf("want size-mismatch, got %v", err)
	}
	if len(ve.Inputs) != 2 || ve.Inputs[1] != 1 {
		t.Fatalf("error does not locate the offending input: %+v", ve)
	}
}

func TestValidateChannelShortage(t *testing.T) {
	a := NewImage(8, 8, 1, Linear, Direction{})
	err := ValidateInputs([]*Image{a}, []int{3})
	ve, ok := err.(*ValidationError)
	if !ok || ve.Kind != ErrChannelMismatch {
		t.Fatalf("want channel-mismatch, got %v", err)
	}
}

func TestValidateDirectionConflict(t *testing.T) {
	a := NewImage(8, 8, 1, Linear, Direction{OriginTopLeft: true})
	b := NewImage(8, 8, 1, Linear, Direction{OriginTopLeft: false})
	err := ValidateInputs([]*Image{a, b}, nil)
	ve, ok := err.(*ValidationError)
	if !ok || ve.Kind != ErrDirectionConflict {
		t.Fatalf("want direction-conflict, got %v", err)
	}
}

func TestValidateNilAndEmpty(t *testing.T) {
	if err := ValidateInputs(nil, nil); err == nil {
		t.Fatal("empty batch: want error")
	}
	a := NewImage(8, 8, 1, Linear, Direction{})
	if err := ValidateInputs([]*Image{a, nil}, nil); err == nil {
		t.Fatal("nil image: want error")
	}
}
