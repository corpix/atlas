package hash

import (
	"hash"
	"hash/fnv"
)

type Hash64 = hash.Hash64

var FNV64a = fnv.New64a
