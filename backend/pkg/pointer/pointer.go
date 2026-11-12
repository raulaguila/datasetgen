package pointer

// Of returns a pointer to v.
func Of[T any](v T) *T {
	return &v
}
