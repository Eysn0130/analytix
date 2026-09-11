package filetools

func PositiveLimit(value any, fallback int, max int) int {
	limit, ok := numericAny(value)
	if !ok || limit <= 0 {
		limit = fallback
	}
	if max > 0 && limit > max {
		limit = max
	}
	return limit
}

func BoundedNonNegativeInteger(value any, fallback int, max int) int {
	out, ok := numericAny(value)
	if !ok || out < 0 {
		out = fallback
	}
	if max > 0 && out > max {
		out = max
	}
	return out
}
