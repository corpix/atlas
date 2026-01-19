package app

import (
	"github.com/urfave/cli/v2"
)

type (
	Flag             = cli.Flag
	GenericFlag      = cli.GenericFlag
	StringFlag       = cli.StringFlag
	PathFlag         = cli.PathFlag
	DurationFlag     = cli.DurationFlag
	BoolFlag         = cli.BoolFlag
	Float64Flag      = cli.Float64Flag
	Float64SliceFlag = cli.Float64SliceFlag
	IntFlag          = cli.IntFlag
	IntSliceFlag     = cli.IntSliceFlag
	Int64Flag        = cli.Int64Flag
	Int64SliceFlag   = cli.Int64SliceFlag
	UintFlag         = cli.UintFlag
	UintSliceFlag    = cli.UintSliceFlag
	Uint64Flag       = cli.Uint64Flag
	Uint64SliceFlag  = cli.Uint64SliceFlag
	Flags            = []Flag
)

const (
	FlagConfig  = "config"
	FlagVerbose = "verbose"
	FlagDebug   = "debug"
)
