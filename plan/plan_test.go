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

type resourceResolver struct {
	requests func(*Task[resource, string, resourceOps]) []resource
	provides func(*Task[resource, string, resourceOps]) []resource
}

func (r resourceResolver) Requests(task *Task[resource, string, resourceOps]) []resource {
	if r.requests == nil {
		return nil
	}
	return r.requests(task)
}

func (r resourceResolver) Provides(task *Task[resource, string, resourceOps]) []resource {
	if r.provides == nil {
		return nil
	}
	return r.provides(task)
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

func TestPlanToposortResolverReceivesTask(t *testing.T) {
	current := []resource{
		{ID: "app", Name: "old", Size: 1},
	}
	next := []resource{
		{ID: "app", Name: "new", Size: 2},
		{ID: "snapshot", Name: "old", Size: 1},
	}

	p := New(resourceOpsEnum, current, next)
	var requested []string
	var provided []string
	resolver := resourceResolver{
		requests: func(task *Task[resource, string, resourceOps]) []resource {
			assert.True(t, task.Plan == p)
			requested = append(requested, string(task.Op)+":"+task.ID+":"+task.Current.Name+"->"+task.Next.Name)
			if task.ID != "app" {
				return nil
			}

			assert.Equal(t, resourceOpsEnum.Update(), task.Op)
			assert.Equal(t, resource{ID: "app", Name: "old", Size: 1}, task.Current)
			assert.Equal(t, resource{ID: "app", Name: "new", Size: 2}, task.Next)
			assert.Equal(t, task.Next, task.Spec)

			return []resource{{Name: task.Current.Name, Size: task.Current.Size}}
		},
		provides: func(task *Task[resource, string, resourceOps]) []resource {
			assert.True(t, task.Plan == p)
			provided = append(provided, string(task.Op)+":"+task.ID)
			return []resource{{Name: task.Spec.Name, Size: task.Spec.Size}}
		},
	}

	tasks, err := p.Toposort(resolver, resourceOpsEnum.Create(), resourceOpsEnum.Update())
	assert.NoError(t, err)
	if assert.Len(t, tasks, 2) {
		assert.Equal(t, "snapshot", tasks[0].ID)
		assert.Equal(t, "app", tasks[1].ID)
	}
	assert.ElementsMatch(t, []string{
		"create:snapshot:->old",
		"update:app:old->new",
	}, requested)
	assert.ElementsMatch(t, []string{
		"create:snapshot",
		"update:app",
	}, provided)
}
