package plan

import (
	"fmt"
	"testing"
)

type benchOp int

const (
	benchOpInvalid benchOp = iota
	benchOpCreate
	benchOpUpdate
	benchOpRead
	benchOpDelete
)

func (o benchOp) Read() benchOp   { return benchOpRead }
func (o benchOp) Create() benchOp { return benchOpCreate }
func (o benchOp) Update() benchOp { return benchOpUpdate }
func (o benchOp) Delete() benchOp { return benchOpDelete }
func (o benchOp) All() []benchOp {
	return []benchOp{benchOpCreate, benchOpUpdate, benchOpRead, benchOpDelete}
}

// benchSpec keeps Equal strictly finer than Identify, which is the property
// provider indexing relies on.
type benchSpec struct {
	id       string
	payload  string
	weight   int64
	requests []*benchSpec
	provides []*benchSpec
}

func (s *benchSpec) String() string   { return s.id }
func (s *benchSpec) Identify() string { return s.id }
func (s *benchSpec) Weight() int64    { return s.weight }

func (s *benchSpec) Equal(v *benchSpec) bool {
	if v == nil {
		return false
	}
	return s.id == v.id && s.payload == v.payload
}

type benchResolver struct{}

func (benchResolver) Requests(s *benchSpec) []*benchSpec { return s.requests }
func (benchResolver) Provides(s *benchSpec) []*benchSpec { return s.provides }

// benchSharedProvider is the agent's route shape: many specs which all depend
// on the same handful of providers, so every request resolves to one of them.
func benchSharedProvider(n int) []*benchSpec {
	link := &benchSpec{id: "link", weight: 2}
	state := &benchSpec{id: "state", weight: 4, requests: []*benchSpec{link}}

	specs := make([]*benchSpec, 0, n+2)
	specs = append(specs, link, state)
	for i := range n {
		specs = append(specs, &benchSpec{
			id:       fmt.Sprintf("route/%d", i),
			weight:   8,
			requests: []*benchSpec{state},
		})
	}
	return specs
}

// benchDistinctProviders gives every request its own provider, which spreads
// the index instead of concentrating it on a few keys.
func benchDistinctProviders(n int) []*benchSpec {
	specs := make([]*benchSpec, 0, n*2)
	for i := range n {
		provider := &benchSpec{id: fmt.Sprintf("provider/%d", i), weight: 2}
		specs = append(specs, provider, &benchSpec{
			id:       fmt.Sprintf("consumer/%d", i),
			weight:   8,
			requests: []*benchSpec{provider},
		})
	}
	return specs
}

func benchmarkToposort(b *testing.B, specs []*benchSpec) {
	b.Helper()

	resolver := benchResolver{}
	for b.Loop() {
		p := New[*benchSpec, string, benchOp](benchOpInvalid, nil, specs)
		if _, err := p.Toposort(resolver); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToposortSharedProvider(b *testing.B) {
	for _, n := range []int{1000, 10000, 25000} {
		specs := benchSharedProvider(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			benchmarkToposort(b, specs)
		})
	}
}

func BenchmarkToposortDistinctProviders(b *testing.B) {
	for _, n := range []int{1000, 10000, 25000} {
		specs := benchDistinctProviders(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			benchmarkToposort(b, specs)
		})
	}
}
