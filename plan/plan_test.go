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
