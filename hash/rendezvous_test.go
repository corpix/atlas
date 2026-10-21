package hash

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

type rendezvousMember struct {
	id     string
	weight float64
}

const (
	rendezvousKeyStride uint64 = 0x9e3779b97f4a7c15
	rendezvousKeyBase   int    = 16
)

var rendezvousByString = NewWeightedRendezvous(
	FNV64a,
	func(digest Hash64, key string, member rendezvousMember) {
		_, _ = digest.Write([]byte(member.id))
		_, _ = digest.Write([]byte(key))
	},
	func(_ string, member rendezvousMember) float64 {
		return member.weight
	},
	func(a, b rendezvousMember) bool {
		return a.id < b.id
	},
)

func rendezvousKey(n int) string {
	return "subject-" + strconv.FormatUint(uint64(n)*rendezvousKeyStride, rendezvousKeyBase)
}

func TestRendezvousPicksCallerValueIndependentOfOrder(t *testing.T) {
	t.Parallel()

	const samples = 32

	var (
		members  = []rendezvousMember{{id: "a", weight: 1}, {id: "b", weight: 1}, {id: "c", weight: 1}}
		reversed = []rendezvousMember{{id: "c", weight: 1}, {id: "b", weight: 1}, {id: "a", weight: 1}}
	)

	for n := range samples {
		key := rendezvousKey(n)
		selected, ok := rendezvousByString.Pick(key, members)
		require.True(t, ok)
		again, ok := rendezvousByString.Pick(key, reversed)
		require.True(t, ok)
		require.Equal(t, selected, again)
	}
}

func TestRendezvousBreaksTiesIndependentOfOrder(t *testing.T) {
	t.Parallel()

	var (
		rendezvous = NewRendezvous(
			FNV64a,
			func(Hash64, string, rendezvousMember) {},
			func(a, b rendezvousMember) bool { return a.id < b.id },
		)
		members = []rendezvousMember{{id: "c"}, {id: "b"}, {id: "a"}}
	)

	selected, ok := rendezvous.Pick("key", members)
	require.True(t, ok)
	require.Equal(t, "a", selected.id)

	reversed := slices.Clone(members)
	slices.Reverse(reversed)
	selected, ok = rendezvous.Pick("key", reversed)
	require.True(t, ok)
	require.Equal(t, "a", selected.id)
}

func TestRendezvousSpreadsKeysAcrossMembers(t *testing.T) {
	t.Parallel()

	const (
		samples              = 90
		minimumKeysPerMember = 10
	)

	var (
		members = []rendezvousMember{{id: "a", weight: 1}, {id: "b", weight: 1}, {id: "c", weight: 1}}
		held    = map[string]int{}
	)

	for n := range samples {
		selected, ok := rendezvousByString.Pick(rendezvousKey(n), members)
		require.True(t, ok)
		held[selected.id]++
	}

	require.Len(t, held, len(members))
	for id, count := range held {
		require.Greater(t, count, minimumKeysPerMember, id)
	}
}

func TestRendezvousKeepsMostKeysWhenMemberLeaves(t *testing.T) {
	t.Parallel()

	const (
		samples          = 200
		maximumMovedKeys = 70
	)

	var (
		before = []rendezvousMember{{id: "a", weight: 1}, {id: "b", weight: 1}, {id: "c", weight: 1}, {id: "d", weight: 1}}
		after  = before[:len(before)-1]
		moved  int
	)

	for n := range samples {
		key := rendezvousKey(n)
		was, ok := rendezvousByString.Pick(key, before)
		require.True(t, ok)
		is, ok := rendezvousByString.Pick(key, after)
		require.True(t, ok)
		if was != is {
			moved++
		}
	}

	require.Less(t, moved, maximumMovedKeys)
}

func TestRendezvousUsesFractionalWeights(t *testing.T) {
	t.Parallel()

	const (
		lightWeight      = 0.25
		heavyWeight      = 2.5
		samples          = 100
		minimumHeavyKeys = 75
	)

	var (
		members = []rendezvousMember{{id: "light", weight: lightWeight}, {id: "heavy", weight: heavyWeight}}
		heavy   int
	)

	for n := range samples {
		selected, ok := rendezvousByString.Pick(rendezvousKey(n), members)
		require.True(t, ok)
		if selected.id == "heavy" {
			heavy++
		}
	}
	require.Greater(t, heavy, minimumHeavyKeys)
}

func TestRendezvousSkipsInvalidWeights(t *testing.T) {
	t.Parallel()

	members := []rendezvousMember{
		{id: "zero"},
		{id: "negative", weight: -1},
		{id: "nan", weight: math.NaN()},
		{id: "positive-infinity", weight: math.Inf(1)},
		{id: "negative-infinity", weight: math.Inf(-1)},
	}

	selected, ok := rendezvousByString.Pick("key", append(members, rendezvousMember{id: "valid", weight: 1}))
	require.True(t, ok)
	require.Equal(t, "valid", selected.id)

	_, ok = rendezvousByString.Pick("key", members)
	require.False(t, ok)
}
