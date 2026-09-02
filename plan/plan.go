package plan

import (
	"container/heap"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

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
	Resolver[T Spec[K, T], K comparable] interface {
		Requests(spec T) []T
		Provides(spec T) []T
	}

	Graph[T Spec[K, T], K comparable, O Ops[O]] struct {
		tasks       Tasks[T, K, O]
		adj         []map[int]void
		indegree    []int
		pos         []int
		unsatisfied []T
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
	Stat[O comparable] struct {
		Counters         map[O]int
		GraphDuration    time.Duration
		ToposortDuration time.Duration
	}
	Ops[O comparable] interface { // fixme: get rid of that, this is overcomplication and I don't like it, could we use predefined consts?
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

// readyQueue pops the ready task with the lowest input position, which is the
// order the previous re-sort of the whole ready slice produced.
type readyQueue struct {
	items []int
	pos   []int
}

func (q *readyQueue) Len() int           { return len(q.items) }
func (q *readyQueue) Less(i, j int) bool { return q.pos[q.items[i]] < q.pos[q.items[j]] }
func (q *readyQueue) Swap(i, j int)      { q.items[i], q.items[j] = q.items[j], q.items[i] }

func (q *readyQueue) Push(x any) {
	q.items = append(q.items, x.(int))
}

func (q *readyQueue) Pop() any {
	old := q.items
	n := len(old)
	item := old[n-1]
	q.items = old[:n-1]
	return item
}

func (g *Graph[T, K, O]) Toposort() (Tasks[T, K, O], error) {
	if len(g.tasks) == 0 {
		return g.tasks, nil
	}

	ready := &readyQueue{pos: g.pos}
	for i := range g.tasks {
		if g.indegree[i] == 0 {
			ready.items = append(ready.items, i)
		}
	}
	heap.Init(ready)

	out := make(Tasks[T, K, O], 0, len(g.tasks))
	for ready.Len() > 0 {
		curr := heap.Pop(ready).(int)
		out = append(out, g.tasks[curr])

		for next := range g.adj[curr] {
			g.indegree[next]--
			if g.indegree[next] == 0 {
				heap.Push(ready, next)
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

// Unsatisfied reports the requests which no task in the plan supplies. A plan is
// a delta against a live system, so this is not an error on its own: whether such
// a request is expected to be met outside of the plan is for the caller to say.
func (g *Graph[T, K, O]) Unsatisfied() []T {
	return g.unsatisfied
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
	res := make([]string, 0, len(s.Counters))
	for k, v := range s.Counters {
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

	var res Tasks[T, K, O]
	for _, op := range ops {
		res = append(res, p.tasksByOp[op]...)
	}
	return res
}

func (p *Plan[T, K, O]) graph(resolver Resolver[T, K], ops ...O) (*Graph[T, K, O], error) {
	graph, err := p.Graph(resolver, ops...)
	if err != nil {
		return nil, err
	}
	return graph, nil
}

func (p *Plan[T, K, O]) Toposort(resolver Resolver[T, K], ops ...O) (Tasks[T, K, O], error) {
	var (
		started = time.Now()
		g, err  = p.graph(resolver, ops...)
	)
	p.stat.GraphDuration = time.Since(started)
	if err != nil {
		return nil, err
	}
	started = time.Now()
	tasks, err := g.Toposort()
	p.stat.ToposortDuration = time.Since(started)
	return tasks, err
}

func (p *Plan[T, K, O]) Graphviz(resolver Resolver[T, K], ops ...O) (string, error) {
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

// supplyIndex buckets the tasks which supply a spec by that spec identity, so
// a lookup does not scan every task. This relies on Equal implying equal
// Identify: a spec whose equality is looser than its identity would be missed.
func (p *Plan[T, K, O]) supplyIndex(supplies [][]T) map[K][]int {
	index := make(map[K][]int, len(supplies))
	for i := range supplies {
		for _, supplied := range supplies[i] {
			id := supplied.Identify()
			refs := index[id]
			if len(refs) > 0 && refs[len(refs)-1] == i {
				continue
			}
			index[id] = append(refs, i)
		}
	}
	return index
}

// findProvider reports the task which supplies req, or -1 when no task does.
// A plan is a delta against a live system, so a request may well be satisfied
// by something which already exists and never enters the plan.
func (p *Plan[T, K, O]) findProvider(index map[K][]int, provides [][]T, req T) int {
	var (
		bestIdx    = -1
		bestWeight int64
	)
	for _, i := range index[req.Identify()] {
		for _, provided := range provides[i] {
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
	return bestIdx
}

// deps splits the task dependencies into what it starts to depend on (acquires),
// what it stops depending on (releases), what it begins to supply (provides) and
// what it stops supplying (withdraws). A delete releases and withdraws everything,
// an update which drops a reference releases just that reference. A task always
// supplies the entity it carries, so a resolver only names what else it supplies.
func (p *Plan[T, K, O]) deps(resolver Resolver[T, K], task *Task[T, K, O]) (acquires, releases, provides, withdraws []T) {
	var (
		zero         T
		currRequests []T
		currProvides []T
	)
	if task.Current != zero {
		currRequests = resolver.Requests(task.Current)
		currProvides = append([]T{task.Current}, resolver.Provides(task.Current)...)
	}
	if task.Next != zero {
		acquires = resolver.Requests(task.Next)
		provides = append([]T{task.Next}, resolver.Provides(task.Next)...)
	}
	return acquires, p.except(currRequests, acquires), provides, p.except(currProvides, provides)
}

func (p *Plan[T, K, O]) except(specs, except []T) []T {
	if len(specs) == 0 {
		return nil
	}
	if len(except) == 0 {
		return specs
	}

	index := make(map[K]void, len(except))
	for _, spec := range except {
		index[spec.Identify()] = void{}
	}

	out := make([]T, 0, len(specs))
	for _, spec := range specs {
		if _, ok := index[spec.Identify()]; ok {
			continue
		}
		out = append(out, spec)
	}
	return out
}

func (p *Plan[T, K, O]) supplies(specs []T, req T) bool {
	for _, spec := range specs {
		if req.Equal(spec) {
			return true
		}
	}
	return false
}

func (p *Plan[T, K, O]) Graph(resolver Resolver[T, K], ops ...O) (*Graph[T, K, O], error) {
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

	addEdge := func(from, to int) {
		if from == to {
			return
		}
		if adj[from] == nil {
			adj[from] = map[int]void{}
		}
		if _, ok := adj[from][to]; ok {
			return
		}
		adj[from][to] = void{}
		indegree[to]++
	}

	acquires := make([][]T, len(tasks))
	releases := make([][]T, len(tasks))
	provides := make([][]T, len(tasks))
	withdraws := make([][]T, len(tasks))
	for i, task := range tasks {
		acquires[i], releases[i], provides[i], withdraws[i] = p.deps(resolver, task)
	}

	var (
		unsatisfied   []T
		provideIndex  = p.supplyIndex(provides)
		withdrawIndex = p.supplyIndex(withdraws)
	)
	for i := range tasks {
		// what a task starts to depend on has to be supplied before it runs
		for _, req := range acquires[i] {
			providerIdx := p.findProvider(provideIndex, provides, req)
			if providerIdx < 0 {
				unsatisfied = append(unsatisfied, req)
				continue
			}
			addEdge(providerIdx, i)
		}
		// what a task stops depending on may only be withdrawn after it runs,
		// and every consumer has to let go before the supply is taken away
		for _, req := range releases[i] {
			for _, j := range withdrawIndex[req.Identify()] {
				if p.supplies(withdraws[j], req) {
					addEdge(i, j)
				}
			}
		}
	}

	return &Graph[T, K, O]{
		tasks:       tasks,
		adj:         adj,
		indegree:    indegree,
		pos:         pos,
		unsatisfied: unsatisfied,
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
	p.stat.Counters[op]++

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

	// input order decides the order of tasks sharing an operation, so a plan
	// built from the same specs always comes out the same way
	seen := make(map[K]void, len(next))
	for _, spec := range next {
		id := spec.Identify()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = void{}
		currentSpec, ok := currentIndex[id]
		if ok {
			continue
		}
		p.push(p.opsEnum.Create(), id, currentSpec, nextIndex[id])
	}

	seen = make(map[K]void, len(current))
	for _, spec := range current {
		id := spec.Identify()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = void{}
		var op O
		nextSpec, ok := nextIndex[id]
		currentSpec := currentIndex[id]
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
		stat: Stat[O]{
			Counters: map[O]int{},
		},
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
