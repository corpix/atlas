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

func Print(xs ...any) {
	dumper.Dump(xs...)
}

func Printf(format string, xs ...any) {
	_, _ = dumper.Printf(format, xs...)
}

func Diff(a, b any) {
	fmt.Println(SDiff(a, b))
}

func SDiff(a, b any) string {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:       difflib.SplitLines(dumper.Sdump(a)),
		B:       difflib.SplitLines(dumper.Sdump(b)),
		Context: 1,
	})

	return diff
}

func Sprintf(format string, xs ...any) string {
	return dumper.Sprintf(format, xs...)
}
