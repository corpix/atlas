package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"git.tatikoma.dev/corpix/atlas/dump"
)

type (
	Plan[T Spec[K, T], K comparable, O Ops[O]] struct {
		opsEnum    O
		tasksByOp  TaskGroups[T, K, O]
		tasksIndex TaskIndex[T, K, O]
		stat       Stat[O]
		current    []T
		next       []T
		diff       Diff[T, K, O]
		changes    int
	}
	Spec[K comparable, T any] interface {
		comparable
		String() string
		Identify() K
		Equal(T) bool
		Weight() int64
	}
	Resolver[T Spec[K, T], K comparable, O Ops[O]] interface {
		Requests(op O, spec T) []T
		Provides(op O, spec T) []T
	}

	Graph[T Spec[K, T], K comparable, O Ops[O]] struct {
		tasks    Tasks[T, K, O]
		adj      []map[int]void
		indegree []int
		pos      []int
	}

	TaskGroups[T Spec[K, T], K comparable, O Ops[O]] map[O][]*Task[T, K, O]
	TaskIndex[T Spec[K, T], K comparable, O Ops[O]]  map[K]*Task[T, K, O]
	Tasks[T Spec[K, T], K comparable, O Ops[O]]      []*Task[T, K, O]
	Task[T Spec[K, T], K comparable, O Ops[O]]       struct {
		ID K
		Op O
		// Spec depends on context.
		// It will contain `next` spec for entity in case create/update operation should be applied
		// according to `plan`, but for read/delete it will contain `current` spec.
		Plan    *Plan[T, K, O]
		Spec    T
		Current T
		Next    T
	}
	Stat[O comparable] map[O]int
	Ops[O comparable]  interface { // fixme: get rid of that, this is overcomplication and I don't like it, could we use predefined consts?
		comparable
		Read() O
		Create() O
		Update() O
		Delete() O

		All() []O
	}

	DiffRecord[T Spec[K, T], K comparable, O Ops[O]] struct {
		Op      O
		Current T
		Next    T
	}
	Diff[T Spec[K, T], K comparable, O Ops[O]]       []DiffRecord[T, K, O]
	DiffFilter[T Spec[K, T], K comparable, O Ops[O]] func(DiffRecord[T, K, O]) bool

	Context[T Spec[K, T], K comparable, Y any] struct {
		Current T
		Next    T
		Data    Y
	}

	void = struct{}
)

func DiffFilterOp[T Spec[K, T], K comparable, O Ops[O]](ops ...O) DiffFilter[T, K, O] {
	return func(record DiffRecord[T, K, O]) bool {
		for _, op := range ops {
			if record.Op == op {
				return true
			}
		}
		return false
	}
}

func (g *Graph[T, K, O]) Toposort() (Tasks[T, K, O], error) {
	if len(g.tasks) == 0 {
		return g.tasks, nil
	}

	ready := make([]int, 0, len(g.tasks))
	for i := range g.tasks {
		if g.indegree[i] == 0 {
			ready = append(ready, i)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		return g.pos[ready[i]] < g.pos[ready[j]]
	})

	out := make(Tasks[T, K, O], 0, len(g.tasks))
	for len(ready) > 0 {
		curr := ready[0]
		ready = ready[1:]
		out = append(out, g.tasks[curr])

		for next := range g.adj[curr] {
			g.indegree[next]--
			if g.indegree[next] == 0 {
				ready = append(ready, next)
				sort.Slice(ready, func(i, j int) bool {
					return g.pos[ready[i]] < g.pos[ready[j]]
				})
			}
		}
	}

	if len(out) != len(g.tasks) {
		var unresolved []string
		for i, deg := range g.indegree {
			if deg > 0 {
				unresolved = append(unresolved, g.tasks[i].String())
			}
		}
		sort.Strings(unresolved)
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(unresolved, ", "))
	}

	return out, nil
}

func (g *Graph[T, K, O]) nodeID(task *Task[T, K, O]) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v|%v", task.Op, task.Spec.Identify())))
	return "n" + hex.EncodeToString(sum[:])
}

func (g *Graph[T, K, O]) label(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
	)
	return replacer.Replace(s)
}

func (g *Graph[T, K, O]) String() string {
	var b strings.Builder
	b.WriteString("digraph plan {\n")

	ordered, err := g.Toposort()
	if err == nil {
		nodeIDs := make(map[*Task[T, K, O]]string, len(g.tasks))
		for _, task := range g.tasks {
			nodeIDs[task] = g.nodeID(task)
		}

		for _, task := range ordered {
			label := g.label(fmt.Sprintf("%v\n%v", task.Op, task.Spec.String()))
			b.WriteString("  ")
			b.WriteString(nodeIDs[task])
			b.WriteString(" [label=\"")
			b.WriteString(label)
			b.WriteString("\"];\n")
		}

		for i, edges := range g.adj {
			if len(edges) == 0 {
				continue
			}
			consumers := make([]int, 0, len(edges))
			for idx := range edges {
				consumers = append(consumers, idx)
			}
			sort.Slice(consumers, func(a, b int) bool {
				return g.pos[consumers[a]] < g.pos[consumers[b]]
			})

			for _, j := range consumers {
				b.WriteString("  ")
				b.WriteString(nodeIDs[g.tasks[i]])
				b.WriteString(" -> ")
				b.WriteString(nodeIDs[g.tasks[j]])
				b.WriteString(";\n")
			}
		}
	}

	b.WriteString("}\n")

	return b.String()
}

func (t Task[T, K, O]) String() string {
	return fmt.Sprintf("%v(%v)", t.Op, t.Spec.String())
}

func (ts Tasks[T, K, O]) String() string {
	res := make([]string, 0, len(ts))
	for _, t := range ts {
		res = append(res, t.String())
	}
	return "[" + strings.Join(res, ",") + "]"
}

func (s Stat[O]) String() string {
	res := make([]string, 0, len(s))
	for k, v := range s {
		res = append(res, fmt.Sprintf("%v:%d", k, v))
	}
	sort.Strings(res)
	return "[" + strings.Join(res, ",") + "]"
}

func (p *Plan[T, K, O]) Transition(next []T) *Plan[T, K, O] {
	if p == nil {
		var opsEnum O
		return New(opsEnum, nil, next)
	}
	return New(p.opsEnum, p.next, next)
}

func (p *Plan[T, K, O]) Task(id K) (*Task[T, K, O], bool) {
	t, ok := p.tasksIndex[id]
	if !ok {
		return nil, false
	}

	return t, true
}

func (p *Plan[T, K, O]) Stat() (int, Stat[O]) {
	return p.changes, p.stat
}

func (p *Plan[T, K, O]) Changes() int {
	return p.changes
}

func (p *Plan[T, K, O]) Tasks(ops ...O) Tasks[T, K, O] {
	if len(ops) == 0 {
		ops = p.opsEnum.All()
	}

	var (
		res      Tasks[T, K, O]
		opDelete = p.opsEnum.Delete()
	)
	// fixme: apply toposort here
	for _, op := range ops {
		tasks := p.tasksByOp[op]
		switch op { // note: change sorting order for operations which should (for example) run backwards (like delete)
		case opDelete:
			reversedTasks := make(Tasks[T, K, O], len(tasks))
			j := len(tasks) - 1
			for n, task := range tasks {
				reversedTasks[j-n] = task
			}
			res = append(res, reversedTasks...)
		default:
			res = append(res, tasks...)
		}
	}
	return res
}

func (p *Plan[T, K, O]) graph(resolver Resolver[T, K, O], ops ...O) (*Graph[T, K, O], error) {
	graph, err := p.Graph(resolver, ops...)
	if err != nil {
		return nil, err
	}
	return graph, nil
}

func (p *Plan[T, K, O]) Toposort(resolver Resolver[T, K, O], ops ...O) (Tasks[T, K, O], error) {
	g, err := p.graph(resolver, ops...)
	if err != nil {
		return nil, err
	}
	return g.Toposort()
}

func (p *Plan[T, K, O]) Graphviz(resolver Resolver[T, K, O], ops ...O) (string, error) {
	g, err := p.graph(resolver, ops...)
	if err != nil {
		return "", err
	}
	return g.String(), nil
}

func (p Plan[T, K, O]) Current() []T {
	return p.current
}

func (p Plan[T, K, O]) Next() []T {
	return p.next
}

func (p Plan[T, K, O]) String() string {
	return p.Tasks().String()
}

func (p Plan[T, K, O]) Diff(filters ...DiffFilter[T, K, O]) string {
	var (
		s     string
		empty T
	)
outer:
	for _, r := range p.diff {
		for _, filter := range filters {
			if !filter(r) {
				continue outer
			}
		}

		s += dump.Sdiff(
			r.Current, r.Next,
			func(p *dump.DiffParameters) {
				p.FromFile = fmt.Sprintf("current:\t%v", r.Current)
				p.ToFile = fmt.Sprintf("next:\t%v", r.Next)
				op := fmt.Sprint(r.Op)
				if r.Current != empty {
					p.FromDate = op
				}
				if r.Next != empty {
					p.ToDate = op
				}
			},
		)
	}
	return s
}

func (p *Plan[T, K, O]) findProvider(tasks Tasks[T, K, O], resolver Resolver[T, K, O], req T) (int, error) {
	var (
		bestIdx    = -1
		bestWeight int64
	)
	for i, task := range tasks {
		provides := resolver.Provides(task.Op, task.Spec)
		for _, provided := range provides {
			if !req.Equal(provided) {
				continue
			}
			weight := provided.Weight()
			if bestIdx == -1 || weight > bestWeight {
				bestIdx = i
				bestWeight = weight
			}
		}
	}

	if bestIdx == -1 {
		return -1, fmt.Errorf("dependency not satisfied: %v", req.String())
	}

	return bestIdx, nil
}

func (p *Plan[T, K, O]) Graph(resolver Resolver[T, K, O], ops ...O) (*Graph[T, K, O], error) {
	tasks := p.Tasks(ops...)
	if len(tasks) == 0 {
		return &Graph[T, K, O]{
			tasks: tasks,
		}, nil
	}

	adj := make([]map[int]void, len(tasks))
	indegree := make([]int, len(tasks))
	pos := make([]int, len(tasks))
	for i := range tasks {
		pos[i] = i
	}

	for i, task := range tasks {
		requests := resolver.Requests(task.Op, task.Spec)
		for _, req := range requests {
			providerIdx, err := p.findProvider(tasks, resolver, req)
			if err != nil {
				return nil, err
			}
			if providerIdx == i {
				continue
			}
			if adj[providerIdx] == nil {
				adj[providerIdx] = map[int]void{}
			}
			if _, ok := adj[providerIdx][i]; ok {
				continue
			}
			adj[providerIdx][i] = void{}
			indegree[i]++
		}
	}

	return &Graph[T, K, O]{
		tasks:    tasks,
		adj:      adj,
		indegree: indegree,
		pos:      pos,
	}, nil
}

func (p Plan[T, K, O]) index(current, next []T) (map[K]T, map[K]T) {
	currentIndex := map[K]T{}
	nextIndex := map[K]T{}

	for _, currentSpec := range current {
		id := currentSpec.Identify()
		indexedSpec, ok := currentIndex[id]
		if !ok || currentSpec.Weight() > indexedSpec.Weight() {
			currentIndex[id] = currentSpec
		}
	}
	for _, nextSpec := range next {
		id := nextSpec.Identify()
		indexedSpec, ok := nextIndex[id]
		if !ok || nextSpec.Weight() > indexedSpec.Weight() {
			nextIndex[id] = nextSpec
		}
	}

	return currentIndex, nextIndex
}

func (p *Plan[T, K, O]) push(op O, id K, current T, next T) {
	p.stat[op]++

	task := &Task[T, K, O]{
		ID:      id,
		Op:      op,
		Plan:    p,
		Current: current,
		Next:    next,
	}

	switch op {
	case p.opsEnum.Create(), p.opsEnum.Update():
		p.changes++
		task.Spec = next
	case p.opsEnum.Delete():
		p.changes++
		task.Spec = current
	case p.opsEnum.Read():
		task.Spec = next
	}

	p.tasksByOp[op] = append(p.tasksByOp[op], task)
	p.tasksIndex[id] = task
	p.diff = append(p.diff, DiffRecord[T, K, O]{
		Op:      op,
		Current: current,
		Next:    next,
	})
}

func (p *Plan[T, K, O]) build(current, next []T) {
	currentIndex, nextIndex := p.index(current, next)
	for id, nextSpec := range nextIndex {
		currentSpec, ok := currentIndex[id]
		if !ok {
			p.push(p.opsEnum.Create(), id, currentSpec, nextSpec)
		}
	}
	for id, currentSpec := range currentIndex {
		var op O
		nextSpec, ok := nextIndex[id]
		if ok {
			if currentSpec.Equal(nextSpec) {
				op = p.opsEnum.Read()
			} else {
				op = p.opsEnum.Update()
			}
		} else {
			op = p.opsEnum.Delete()
		}
		p.push(op, id, currentSpec, nextSpec)
	}
}

func New[T Spec[K, T], K comparable, O Ops[O]](_ O, current, next []T) *Plan[T, K, O] {
	plan := &Plan[T, K, O]{
		current:    current,
		next:       next,
		tasksByOp:  TaskGroups[T, K, O]{},
		tasksIndex: TaskIndex[T, K, O]{},
		stat:       Stat[O]{},
	}
	plan.build(current, next)

	return plan
}

func TaskContext[T Spec[K, T], K comparable, O Ops[O], Y any](t *Task[T, K, O], data Y) Context[T, K, Y] {
	return Context[T, K, Y]{
		Current: t.Current,
		Next:    t.Next,
		Data:    data,
	}
}
