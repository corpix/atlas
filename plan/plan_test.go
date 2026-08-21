package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type resourceOps string

var resourceOpsEnum resourceOps

func (o resourceOps) Read() resourceOps   { return "read" }
func (o resourceOps) Create() resourceOps { return "create" }
func (o resourceOps) Update() resourceOps { return "update" }
func (o resourceOps) Delete() resourceOps { return "delete" }
func (o resourceOps) All() []resourceOps {
	return []resourceOps{o.Read(), o.Create(), o.Update(), o.Delete()}
}

type resource struct {
	ID   string
	Name string
	Size int
}

func (r resource) String() string {
	return r.Identify()
}

func (r resource) Identify() string {
	return r.ID
}

func (r resource) Equal(other resource) bool {
	return r.Name == other.Name && r.Size == other.Size
}

func (r resource) Weight() int64 {
	return 0
}

// ref names a resource without its identity, so a dependent may ask for it
// without knowing which entity supplies it
func (r resource) ref() resource {
	return resource{Name: r.Name, Size: r.Size}
}

type resourceResolver struct {
	requests func(resource) []resource
	provides func(resource) []resource
}

func (r resourceResolver) Requests(spec resource) []resource {
	if r.requests == nil {
		return nil
	}
	return r.requests(spec)
}

func (r resourceResolver) Provides(spec resource) []resource {
	if r.provides == nil {
		return nil
	}
	return r.provides(spec)
}

func (ts Tasks[T, K, O]) ids() []K {
	out := make([]K, len(ts))
	for i, task := range ts {
		out[i] = task.ID
	}
	return out
}

func (ts Tasks[T, K, O]) positions() map[K]int {
	out := make(map[K]int, len(ts))
	for i, task := range ts {
		out[task.ID] = i
	}
	return out
}

func TestPlan(t *testing.T) {
	type plan = Plan[resource, string, resourceOps]
	current := []resource{
		{ID: "a", Name: "alpha", Size: 1},
		{ID: "b", Name: "beta", Size: 2},
		{ID: "c", Name: "gamma", Size: 3},
	}
	next := []resource{
		{ID: "a", Name: "alpha", Size: 1},
		{ID: "b", Name: "delta", Size: 4},
		{ID: "d", Name: "epsilon", Size: 5},
	}
	test := func(t *testing.T, p *plan) {
		t.Run("creates new entities", func(t *testing.T) {
			tasks := p.Tasks(resourceOpsEnum.Create())
			assert.Len(t, tasks, 1)
			assert.Equal(t, "d", tasks[0].ID)
		})

		t.Run("deletes absent entities", func(t *testing.T) {
			tasks := p.Tasks(resourceOpsEnum.Delete())
			assert.Len(t, tasks, 1)
			assert.Equal(t, "c", tasks[0].ID)
		})

		t.Run("updates changed entities", func(t *testing.T) {
			tasks := p.Tasks(resourceOpsEnum.Update())
			assert.Len(t, tasks, 1)
			assert.Equal(t, "b", tasks[0].ID)
		})

		t.Run("reads unchanged entities", func(t *testing.T) {
			tasks := p.Tasks(resourceOpsEnum.Read())
			assert.Len(t, tasks, 1)
			assert.Equal(t, "a", tasks[0].ID)
		})

		t.Run("checks overall stats", func(t *testing.T) {
			changes, stat := p.Stat()
			assert.Equal(t, 3, changes)
			assert.Equal(t, 1, stat[resourceOpsEnum.Create()])
			assert.Equal(t, 1, stat[resourceOpsEnum.Update()])
			assert.Equal(t, 1, stat[resourceOpsEnum.Delete()])
			assert.Equal(t, 1, stat[resourceOpsEnum.Read()])
		})
	}

	t.Run("straight_forward", func(t *testing.T) {
		p := New(resourceOpsEnum, current, next)
		test(t, p)
	})
	t.Run("transitions", func(t *testing.T) {
		var sp *plan
		sp = sp.Transition(current)
		assert.Len(t, sp.Tasks(resourceOpsEnum.Create()), len(current))
		sp = sp.Transition(next)
		test(t, sp)
	})
}

func TestPlanToposortResolverReceivesSpecs(t *testing.T) {
	current := []resource{
		{ID: "app", Name: "old", Size: 1},
	}
	next := []resource{
		{ID: "app", Name: "new", Size: 2},
		{ID: "snapshot", Name: "kept", Size: 1},
	}

	var seen []string
	resolver := resourceResolver{
		requests: func(spec resource) []resource {
			seen = append(seen, spec.ID+":"+spec.Name)
			if spec.ID == "app" && spec.Name == "new" {
				return []resource{{Name: "kept", Size: 1}}
			}
			return nil
		},
	}

	p := New(resourceOpsEnum, current, next)
	tasks, err := p.Toposort(resolver, resourceOpsEnum.Create(), resourceOpsEnum.Update())
	assert.NoError(t, err)
	if assert.Len(t, tasks, 2) {
		assert.Equal(t, "snapshot", tasks[0].ID)
		assert.Equal(t, "app", tasks[1].ID)
	}
	assert.ElementsMatch(t, []string{
		"app:old",
		"app:new",
		"snapshot:kept",
	}, seen, "both sides of a changing entity have to be resolved")
}

func TestPlanToposortDeletesConsumersBeforeSuppliers(t *testing.T) {
	volume := resource{ID: "volume", Name: "vol", Size: 1}
	first := resource{ID: "first", Name: "first", Size: 2}
	second := resource{ID: "second", Name: "second", Size: 3}

	resolver := resourceResolver{
		requests: func(spec resource) []resource {
			if spec.ID == volume.ID {
				return nil
			}
			return []resource{volume.ref()}
		},
	}

	p := New(resourceOpsEnum, []resource{volume, first, second}, nil)
	tasks, err := p.Toposort(resolver)
	assert.NoError(t, err)

	pos := tasks.positions()
	assert.Less(t, pos["first"], pos["volume"],
		"a supplier may only be deleted once every consumer is gone: %v", tasks.ids())
	assert.Less(t, pos["second"], pos["volume"],
		"a supplier may only be deleted once every consumer is gone: %v", tasks.ids())
}

func TestPlanToposortUpdateReleasesBeforeDelete(t *testing.T) {
	oldVolume := resource{ID: "old", Name: "v1", Size: 1}
	newVolume := resource{ID: "new", Name: "v2", Size: 2}
	appCurrent := resource{ID: "app", Name: "app", Size: 1}
	appNext := resource{ID: "app", Name: "app", Size: 2}

	resolver := resourceResolver{
		requests: func(spec resource) []resource {
			if spec.ID != appCurrent.ID {
				return nil
			}
			if spec.Size == appCurrent.Size {
				return []resource{oldVolume.ref()}
			}
			return []resource{newVolume.ref()}
		},
	}

	p := New(resourceOpsEnum,
		[]resource{appCurrent, oldVolume},
		[]resource{appNext, newVolume},
	)
	tasks, err := p.Toposort(resolver)
	assert.NoError(t, err)

	pos := tasks.positions()
	assert.Less(t, pos["new"], pos["app"],
		"the supplier it moves to has to exist first: %v", tasks.ids())
	assert.Less(t, pos["app"], pos["old"],
		"the supplier it moves off may only be deleted once it has let go: %v", tasks.ids())
}

func TestPlanToposortMissingSupplier(t *testing.T) {
	app := resource{ID: "app", Name: "app", Size: 1}
	resolver := resourceResolver{
		requests: func(resource) []resource {
			return []resource{{Name: "absent", Size: 9}}
		},
	}

	p := New(resourceOpsEnum, nil, []resource{app})
	tasks, err := p.Toposort(resolver)
	assert.NoError(t, err, "a plan is a delta, a request may be satisfied outside of it")
	assert.Equal(t, []string{"app"}, tasks.ids())

	graph, err := p.Graph(resolver)
	assert.NoError(t, err)
	assert.Equal(t, []resource{{Name: "absent", Size: 9}}, graph.Unsatisfied(),
		"the caller has to be able to tell whether it expected the request to be met outside the plan")
}

func TestPlanTasksOrderIsStable(t *testing.T) {
	volume := resource{ID: "volume", Name: "vol", Size: 1}
	current := []resource{
		volume,
		{ID: "first", Name: "first", Size: 2},
		{ID: "second", Name: "second", Size: 3},
		{ID: "third", Name: "third", Size: 4},
	}
	next := []resource{
		{ID: "fourth", Name: "fourth", Size: 5},
		{ID: "fifth", Name: "fifth", Size: 6},
	}

	resolver := resourceResolver{
		requests: func(spec resource) []resource {
			if spec.ID == volume.ID {
				return nil
			}
			return []resource{volume.ref()}
		},
	}

	want, err := New(resourceOpsEnum, current, next).Toposort(resolver)
	assert.NoError(t, err)

	for range 32 {
		got, err := New(resourceOpsEnum, current, next).Toposort(resolver)
		assert.NoError(t, err)
		assert.Equal(t, want.ids(), got.ids(),
			"the same entities have to plan into the same order")
	}
}
