package seq

type Batcher[T any] struct {
	items []T
	size  int
}

func (b *Batcher[T]) Iter() func(yield func([]T) bool) {
	return func(yield func([]T) bool) {
		if b.size <= 0 {
			return
		}

		items := b.items
		for len(items) > 0 {
			end := min(b.size, len(items))

			batch := items[:end]
			items = items[end:]

			if !yield(batch) {
				return
			}
		}
	}
}

func (b *Batcher[T]) Len() int {
	if b.size <= 0 {
		return 0
	}
	return (len(b.items) + b.size - 1) / b.size
}

func NewBatcher[T any](items []T, size int) *Batcher[T] {
	return &Batcher[T]{
		items: items,
		size:  size,
	}
}
