package seq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBatcher(t *testing.T) {
	testCases := []struct {
		name        string
		items       []int
		expected    [][]int
		batchSize   int
		expectedLen int
	}{
		{
			name:        "Evenly divisible",
			items:       []int{1, 2, 3, 4, 5, 6},
			batchSize:   2,
			expected:    [][]int{{1, 2}, {3, 4}, {5, 6}},
			expectedLen: 3,
		},
		{
			name:        "Unevenly divisible",
			items:       []int{1, 2, 3, 4, 5},
			batchSize:   2,
			expected:    [][]int{{1, 2}, {3, 4}, {5}},
			expectedLen: 3,
		},
		{
			name:        "Batch size larger than items",
			items:       []int{1, 2, 3},
			batchSize:   5,
			expected:    [][]int{{1, 2, 3}},
			expectedLen: 1,
		},
		{
			name:        "Empty items",
			items:       []int{},
			batchSize:   2,
			expected:    nil,
			expectedLen: 0,
		},
		{
			name:        "Zero batch size",
			items:       []int{1, 2, 3},
			batchSize:   0,
			expected:    nil,
			expectedLen: 0,
		},
		{
			name:        "Negative batch size",
			items:       []int{1, 2, 3},
			batchSize:   -1,
			expected:    nil,
			expectedLen: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batcher := NewBatcher(tc.items, tc.batchSize)

			assert.Equal(t, tc.expectedLen, batcher.Len())

			var result [][]int
			iter := batcher.Iter()

			iter(func(batch []int) bool {
				val := make([]int, len(batch))
				copy(val, batch)
				result = append(result, val)
				return true
			})

			assert.Equal(t, tc.expected, result)
		})
	}
}
