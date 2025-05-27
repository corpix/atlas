package iter

// Batches returns an iterator that yields batches of items from a slice.
func Batches[T any](items []T, batchSize int) func(yield func([]T) bool) {
	return func(yield func([]T) bool) {
		if batchSize <= 0 {
			return
		}

		for len(items) > 0 {
			end := min(batchSize, len(items))

			batch := items[:end]
			items = items[end:]

			if !yield(batch) {
				return
			}
		}
	}
}
