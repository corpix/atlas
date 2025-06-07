package iter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBatches(t *testing.T) {
	testCases := []struct {
		name      string
		items     []int
		expected  [][]int
		batchSize int
	}{
		{
			name:      "Evenly divisible",
			items:     []int{1, 2, 3, 4, 5, 6},
			batchSize: 2,
			expected:  [][]int{{1, 2}, {3, 4}, {5, 6}},
		},
		{
			name:      "Unevenly divisible",
			items:     []int{1, 2, 3, 4, 5},
			batchSize: 2,
			expected:  [][]int{{1, 2}, {3, 4}, {5}},
		},
		{
			name:      "Batch size larger than items",
			items:     []int{1, 2, 3},
			batchSize: 5,
			expected:  [][]int{{1, 2, 3}},
		},
		{
			name:      "Empty items",
			items:     []int{},
			batchSize: 2,
			expected:  nil,
		},
		{
			name:      "Zero batch size",
			items:     []int{1, 2, 3},
			batchSize: 0,
			expected:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result [][]int
			iter := Batches(tc.items, tc.batchSize)

			for batch := range iter {
				val := make([]int, len(batch))
				copy(val, batch)
				result = append(result, val)
			}

			assert.Equal(t, result, tc.expected)
		})
	}
}
