package dump

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/pmezard/go-difflib/difflib"
)

var dumper = spew.ConfigState{
	Indent:                  "  ",
	DisablePointerAddresses: true,
	DisableCapacities:       false,
	SortKeys:                true,
	DisableMethods:          true,
	MaxDepth:                10,
}

type (
	DiffParameters = difflib.UnifiedDiff
	DiffOption     func(*DiffParameters)
)

func Print(xs ...any) {
	dumper.Dump(xs...)
}

func Printf(format string, xs ...any) {
	_, _ = dumper.Printf(format, xs...)
}

func Sprint(xs ...any) string {
	return dumper.Sdump(xs...)
}

func Sprintf(format string, xs ...any) string {
	return dumper.Sprintf(format, xs...)
}

func Diff(a, b any, opts ...DiffOption) {
	fmt.Println(Sdiff(a, b, opts...))
}

func Sdiff(a, b any, opts ...DiffOption) string {
	params := difflib.UnifiedDiff{
		Context: 1,
	}
	for _, fn := range opts {
		fn(&params)
	}
	params.A = difflib.SplitLines(dumper.Sdump(a))
	params.B = difflib.SplitLines(dumper.Sdump(b))
	diff, _ := difflib.GetUnifiedDiffString(params)

	return diff
}
