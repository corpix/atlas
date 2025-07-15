package seq

import (
	"iter"
)

type Set[K comparable, V any] struct {
	id func(V) K
	kv map[K]V
	vn map[K]int
	v  []V
}

func (st *Set[K, V]) Add(ts ...V) {
	for _, t := range ts {
		id := st.id(t)
		n, exists := st.vn[id]
		if exists {
			st.v[n] = t
		} else {
			st.v = append(st.v, t)
			st.vn[id] = len(st.v) - 1
		}

		st.kv[id] = t
	}
}

func (st *Set[K, V]) Del(id K) bool {
	n, exists := st.vn[id]
	if !exists {
		return false
	}

	st.v = append(st.v[:n], st.v[n+1:]...)
	delete(st.kv, id)
	delete(st.vn, id)
	for i := n; i < len(st.v[n:]); i++ {
		st.vn[st.id(st.v[i])] = n + i
	}

	return true
}

func (st *Set[K, V]) Has(id K) bool {
	_, ok := st.vn[id]
	return ok
}

func (st *Set[K, V]) Get(id K) V {
	return st.kv[id]
}

func (st *Set[K, V]) iter(yield func(V) bool) {
	for _, t := range st.v {
		if !yield(t) {
			break
		}
	}
}

func (st *Set[K, V]) Len() int {
	return len(st.v)
}

func (st *Set[K, V]) Iter() iter.Seq[V] {
	return st.iter
}

func (st *Set[K, V]) Copy() *Set[K, V] {
	return NewSet(st.v, st.id)
}

func (st *Set[K, V]) Merge(ss ...*Set[K, V]) *Set[K, V] {
	r := st.Copy()
	for _, set := range ss {
		for v := range set.Iter() {
			r.Add(v)
		}
	}
	return r
}

func (st *Set[K, V]) Difference(s *Set[K, V]) *Set[K, V] {
	r := st.Copy()
	for t := range s.Iter() {
		r.Del(st.id(t))
	}
	return r
}

func (st *Set[K, V]) DifferenceSynchronized(s *Set[K, V]) *Set[K, V] {
	lrDiff := st.Difference(s)
	rlDiff := s.Difference(st)
	return lrDiff.Merge(rlDiff)
}

func NewSet[K comparable, V any](ts []V, id func(V) K) *Set[K, V] {
	st := &Set[K, V]{
		id: id,
		kv: make(map[K]V),
		v:  make([]V, 0, len(ts)),
		vn: make(map[K]int),
	}
	for n, t := range ts {
		k := id(t)
		st.v = append(st.v, t)
		st.vn[k] = n
		st.kv[k] = t
	}
	return st
}
