package tex

import "strconv"

func itoa(v int) string { return strconv.Itoa(v) }

// ftoa formats a float32 with full round-trip precision.
func ftoa(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
