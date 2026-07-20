package yaml

type TagHandler = func(node *Node) error

type Expander struct {
	handlers map[string]TagHandler
}

func (e *Expander) Handle(tag string, handler TagHandler) *Expander {
	e.handlers[tag] = handler
	return e
}

func (e *Expander) Node(node *Node) error {
	handler, ok := e.handlers[node.Tag]
	if ok {
		return handler(node)
	}
	for _, child := range node.Content {
		err := e.Node(child)
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Expander) Bytes(data []byte) ([]byte, error) {
	root := &Node{}
	err := Unmarshal(data, root)
	if err != nil {
		return nil, err
	}
	err = e.Node(root)
	if err != nil {
		return nil, err
	}
	return Marshal(root)
}

type Option = func(*Expander)

func WithHandler(tag string, handler TagHandler) Option {
	return func(e *Expander) {
		e.Handle(tag, handler)
	}
}

func NewExpander(options ...Option) *Expander {
	e := &Expander{handlers: map[string]TagHandler{}}
	for _, option := range options {
		option(e)
	}
	return e
}
