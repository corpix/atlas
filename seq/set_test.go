package seq

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type setTestItem struct {
	Data string
	ID   int
}

func setTestItemID(i setTestItem) int {
	return i.ID
}

func mkSetItem(id int, data string) setTestItem {
	return setTestItem{ID: id, Data: data}
}

func getAllSetItems(st *Set[int, setTestItem]) []setTestItem {
	var items []setTestItem
	if st == nil {
		return items
	}
	for t := range st.Iter() {
		items = append(items, t)
	}
	return items
}

func TestSet(t *testing.T) {
	t.Run("New", func(t *testing.T) {
		st := NewSet(nil, setTestItemID)
		assert.Empty(t, getAllSetItems(st))

		i1, i2 := mkSetItem(1, "a"), mkSetItem(2, "b")
		st = NewSet([]setTestItem{i1, i2}, setTestItemID)
		assert.Equal(t, i1, st.Get(i1.ID))
		assert.Equal(t, i2, st.Get(i2.ID))

		expectedItems := []setTestItem{i1, i2}
		assert.Equal(t, expectedItems, getAllSetItems(st))
	})

	t.Run("AddDel", func(t *testing.T) {
		st := NewSet(nil, setTestItemID)
		i1, i2 := mkSetItem(1, "a"), mkSetItem(2, "b")

		st.Add(i1)
		assert.Equal(t, i1, st.Get(i1.ID))

		st.Add(i2)
		assert.Equal(t, i2, st.Get(i2.ID))

		i1v2 := mkSetItem(1, "a_new")
		st.Add(i1v2)

		assert.Equal(t, i1v2, st.Get(i1v2.ID))

		assert.True(t, st.Del(i2.ID))
		var expectedZero setTestItem
		assert.Equal(t, expectedZero, st.Get(i2.ID))

		assert.True(t, st.Del(i1v2.ID))
		assert.Equal(t, expectedZero, st.Get(i1v2.ID))

		assert.False(t, st.Del(666))
	})

	t.Run("Get", func(t *testing.T) {
		i1 := mkSetItem(10, "z")
		st := NewSet([]setTestItem{i1}, setTestItemID)
		assert.Equal(t, i1, st.Get(10))
		var expectedZero setTestItem
		assert.Equal(t, expectedZero, st.Get(99))
	})

	t.Run("Iter", func(t *testing.T) {
		st := NewSet(nil, setTestItemID)
		cnt := 0
		for range st.Iter() {
			cnt++
		}
		assert.Equal(t, 0, cnt)

		i1, i2 := mkSetItem(1, "a"), mkSetItem(2, "b")
		st = NewSet([]setTestItem{i1, i2}, setTestItemID)

		var res []setTestItem
		for v := range st.Iter() {
			res = append(res, v)
		}
		assert.Equal(t, []setTestItem{i1, i2}, res)

		res = res[:0]
		st.Iter()(func(v setTestItem) bool {
			res = append(res, v)
			return v.ID != i1.ID
		})
		assert.Equal(t, []setTestItem{i1}, res)
	})

	t.Run("Copy", func(t *testing.T) {
		i1, i2 := mkSetItem(1, "a"), mkSetItem(2, "b")
		originalSt := NewSet([]setTestItem{i1, i2}, setTestItemID)
		copiedSt := originalSt.Copy()

		expectedInitial := []setTestItem{i1, i2}
		assert.Equal(t, expectedInitial, getAllSetItems(copiedSt))

		i3 := mkSetItem(3, "c")
		copiedSt.Add(i3)
		assert.Len(t, getAllSetItems(originalSt), 2)
		var expectedZero setTestItem
		assert.Equal(t, expectedZero, originalSt.Get(i3.ID))

		originalSt.Del(i1.ID)

		expectedStateOfCopiedSt := []setTestItem{i1, i2, i3}
		assert.Equal(t, expectedStateOfCopiedSt, getAllSetItems(copiedSt))
		assert.Equal(t, i1, copiedSt.Get(i1.ID))
		assert.Equal(t, i2, copiedSt.Get(i2.ID))
		assert.Equal(t, i3, copiedSt.Get(i3.ID))
	})

	t.Run("Merge", func(t *testing.T) {
		i1, i2, i3 := mkSetItem(1, "a"), mkSetItem(2, "b"), mkSetItem(3, "c")
		i2Updated := mkSetItem(2, "b_updated")
		i4 := mkSetItem(4, "d")

		st1 := NewSet([]setTestItem{i1, i2}, setTestItemID)
		st2 := NewSet([]setTestItem{i2Updated, i3}, setTestItemID)

		mergedSt := st1.Merge(st2)
		expected := []setTestItem{i1, i2Updated, i3}
		assert.Equal(t, expected, getAllSetItems(mergedSt))

		st3 := NewSet([]setTestItem{i4}, setTestItemID)
		mergedSt2 := st1.Merge(st2, st3)
		expected2 := []setTestItem{i1, i2Updated, i3, i4}
		assert.Equal(t, expected2, getAllSetItems(mergedSt2))

		emptySt := NewSet([]setTestItem{}, setTestItemID)
		mergedEmpty := st1.Merge(emptySt)
		assert.Equal(t, getAllSetItems(st1), getAllSetItems(mergedEmpty))

		mergedToEmpty := emptySt.Merge(st1)
		assert.Equal(t, getAllSetItems(st1), getAllSetItems(mergedToEmpty))
	})

	t.Run("Difference", func(t *testing.T) {
		i1, i2, i3 := mkSetItem(1, "a"), mkSetItem(2, "b"), mkSetItem(3, "c")

		st1 := NewSet([]setTestItem{i1, i2, i3}, setTestItemID)
		st2 := NewSet([]setTestItem{i2}, setTestItemID)
		diffSt := st1.Difference(st2)
		expected := []setTestItem{i1, i3}
		assert.Equal(t, expected, getAllSetItems(diffSt))

		st3 := NewSet([]setTestItem{i1, i2, i3}, setTestItemID)
		st4 := NewSet([]setTestItem{i1}, setTestItemID)
		diffSt2 := st3.Difference(st4)
		expected2 := []setTestItem{i2, i3}
		assert.Equal(t, expected2, getAllSetItems(diffSt2))

		st5 := NewSet([]setTestItem{i1, i2, i3}, setTestItemID)
		st6 := NewSet([]setTestItem{i3}, setTestItemID)
		diffSt3 := st5.Difference(st6)
		expected3 := []setTestItem{i1, i2}
		assert.Equal(t, expected3, getAllSetItems(diffSt3))

		st7 := NewSet([]setTestItem{i1, i2}, setTestItemID)
		st8 := NewSet([]setTestItem{i3}, setTestItemID)
		diffSt4 := st7.Difference(st8)
		expected4 := []setTestItem{i1, i2}
		assert.Equal(t, expected4, getAllSetItems(diffSt4))

		syncDiffSame := st1.Difference(st1)
		assert.Len(t, getAllSetItems(syncDiffSame), 0)

		diffNoOp := st1.Difference(NewSet([]setTestItem{}, setTestItemID))
		assert.Equal(t, getAllSetItems(st1), getAllSetItems(diffNoOp))
	})

	t.Run("DifferenceSynchronized", func(t *testing.T) {
		i1, i2, i3 := mkSetItem(1, "a"), mkSetItem(2, "b"), mkSetItem(3, "c")

		st1 := NewSet([]setTestItem{i1, i2}, setTestItemID)
		st2 := NewSet([]setTestItem{i2, i3}, setTestItemID)
		syncDiff := st1.DifferenceSynchronized(st2)
		expected := []setTestItem{i1, i3}
		assert.ElementsMatch(t, expected, getAllSetItems(syncDiff))

		st3 := NewSet([]setTestItem{i1}, setTestItemID)
		st4 := NewSet([]setTestItem{i2}, setTestItemID)
		syncDiff2 := st3.DifferenceSynchronized(st4)
		expected2 := []setTestItem{i1, i2}
		assert.ElementsMatch(t, expected2, getAllSetItems(syncDiff2))

		syncDiffSame := st1.DifferenceSynchronized(st1)
		assert.Len(t, getAllSetItems(syncDiffSame), 0)

		diffNoOp := st1.DifferenceSynchronized(NewSet([]setTestItem{}, setTestItemID))
		assert.Equal(t, getAllSetItems(st1), getAllSetItems(diffNoOp))
	})
}
