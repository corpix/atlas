package io

import "io"

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// NopWriteCloser wraps a Writer with a Close that does nothing, for the writers
// which require a WriteCloser but must not own the underlying writer.
func NopWriteCloser(w io.Writer) io.WriteCloser {
	return nopWriteCloser{w}
}
