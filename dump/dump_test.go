package dump

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test(t *testing.T) {
	type (
		flat struct {
			A string
			B string
		}
		nested struct {
			A flat
			B string
		}
	)
	assert.Equal(t, "[{foo bar}]", Sprintf("%v", flat{A: "foo", B: "bar"}))
	assert.Equal(t, "[{A:foo B:bar}]", Sprintf("%+v", flat{A: "foo", B: "bar"}))
	assert.Equal(t, "[{{foo bar} baz}]", Sprintf("%v", nested{
		A: flat{A: "foo", B: "bar"},
		B: "baz",
	}))
	assert.Equal(t, "[{A:{A:foo B:bar} B:baz}]", Sprintf("%+v", nested{
		A: flat{A: "foo", B: "bar"},
		B: "baz",
	}))

}
