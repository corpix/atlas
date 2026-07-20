package yaml

import (
	"gopkg.in/yaml.v3"
)

type (
	Node        = yaml.Node
	Kind        = yaml.Kind
	Style       = yaml.Style
	Decoder     = yaml.Decoder
	Encoder     = yaml.Encoder
	Marshaler   = yaml.Marshaler
	Unmarshaler = yaml.Unmarshaler
	IsZeroer    = yaml.IsZeroer
	TypeError   = yaml.TypeError
)

const (
	DocumentNode = yaml.DocumentNode
	SequenceNode = yaml.SequenceNode
	MappingNode  = yaml.MappingNode
	ScalarNode   = yaml.ScalarNode
	AliasNode    = yaml.AliasNode
)

const (
	TaggedStyle       = yaml.TaggedStyle
	DoubleQuotedStyle = yaml.DoubleQuotedStyle
	SingleQuotedStyle = yaml.SingleQuotedStyle
	LiteralStyle      = yaml.LiteralStyle
	FoldedStyle       = yaml.FoldedStyle
	FlowStyle         = yaml.FlowStyle
)

var (
	Marshal    = yaml.Marshal
	Unmarshal  = yaml.Unmarshal
	NewDecoder = yaml.NewDecoder
	NewEncoder = yaml.NewEncoder
)
