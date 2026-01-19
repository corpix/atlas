package app

import (
	"os"
)

type (
	Signal           = os.Signal
	Signals          = []Signal
	SignalGroup      uint8
	SignalGroupIndex = map[Signal]SignalGroup
)

const (
	SignalGroupStop   SignalGroup = 0
	SignalGroupNotify             = iota
)

var (
	SignalGroups = []SignalGroup{
		SignalGroupStop,
		SignalGroupNotify,
	}
)

func GroupSignals(s interface{ Signals(...SignalGroup) Signals }) SignalGroupIndex {
	sgids := SignalGroupIndex{}
	for _, sgid := range SignalGroups {
		for _, sig := range s.Signals(sgid) {
			sgids[sig] = sgid
		}
	}
	return sgids
}
