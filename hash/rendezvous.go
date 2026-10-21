package hash

import (
	"math"
)

const (
	rendezvousHashBits       = 64
	rendezvousUniformBits    = 53
	rendezvousBucketMidpoint = 0.5
	rendezvousDefaultWeight  = 1
)

type Rendezvous[K, T any] struct {
	newHash func() Hash64
	write   func(Hash64, K, T)
	weight  func(K, T) float64
	less    func(T, T) bool
}

func (r Rendezvous[K, T]) Pick(key K, members []T) (T, bool) {
	var (
		selected T
		best     float64
		found    bool
		digest   = r.newHash()
	)
	for _, member := range members {
		weight := r.weight(key, member)
		if weight <= 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			continue
		}
		digest.Reset()
		r.write(digest, key, member)
		// midpoints keep the uniform sample strictly between zero and one
		u := math.Ldexp(
			float64(digest.Sum64()>>(rendezvousHashBits-rendezvousUniformBits))+rendezvousBucketMidpoint,
			-rendezvousUniformBits,
		)
		score := math.Log(u) / weight
		if !found || score > best || score == best && r.less(member, selected) {
			selected, best, found = member, score, true
		}
	}
	return selected, found
}

func NewRendezvous[K, T any](
	newHash func() Hash64,
	write func(Hash64, K, T),
	less func(T, T) bool,
) Rendezvous[K, T] {
	return NewWeightedRendezvous(
		newHash,
		write,
		func(_ K, _ T) float64 { return rendezvousDefaultWeight },
		less,
	)
}

func NewWeightedRendezvous[K, T any](
	newHash func() Hash64,
	write func(Hash64, K, T),
	weight func(K, T) float64,
	less func(T, T) bool,
) Rendezvous[K, T] {
	return Rendezvous[K, T]{
		newHash: newHash,
		write:   write,
		weight:  weight,
		less:    less,
	}
}
