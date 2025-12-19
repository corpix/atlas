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
	assert.Equal(t, "{foo bar}", Sprintf("%v", flat{A: "foo", B: "bar"}))
	assert.Equal(t, "{A:foo B:bar}", Sprintf("%+v", flat{A: "foo", B: "bar"}))
	assert.Equal(t, "{{foo bar} baz}", Sprintf("%v", nested{
		A: flat{A: "foo", B: "bar"},
		B: "baz",
	}))
	assert.Equal(t, "{A:{A:foo B:bar} B:baz}", Sprintf("%+v", nested{
		A: flat{A: "foo", B: "bar"},
		B: "baz",
	}))

	assert.Equal(
		t,
		Sdiff(flat{A: "a", B: "b"}, flat{A: "a", B: "bb"}),
		`@@ -2,3 +2,3 @@
   A: (string) (len=1) "a",
-  B: (string) (len=1) "b"
+  B: (string) (len=2) "bb"
 }
`,
	)
	assert.Equal(
		t,
		Sdiff(flat{A: "a", B: "b"}, flat{A: "a", B: "b"}),
		``,
	)
}
