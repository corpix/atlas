package dump

import (
	"github.com/davecgh/go-spew/spew"
)

var dumper = spew.ConfigState{
	SortKeys: true,
}

func Print(xs ...any) {
	dumper.Dump(xs...)
}

func Printf(format string, xs ...any) {
	dumper.Printf(format, xs)
}
