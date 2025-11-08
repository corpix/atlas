package dump

import (
	"github.com/davecgh/go-spew/spew"
)

var dumper = spew.ConfigState{
	SortKeys: true,
	Indent:   "  ",
}

func Print(xs ...any) {
	dumper.Dump(xs...)
}

func Printf(format string, xs ...any) {
	_, _ = dumper.Printf(format, xs...)
}

func Sprintf(format string, xs ...any) string {
	return dumper.Sprintf(format, xs...)
}
