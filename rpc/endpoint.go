package rpc

import (
	"net"
	"strconv"
	"strings"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type Endpoint struct {
	Scheme string
	Host   string
	Port   uint16
	Path   string
}

func (e *Endpoint) Network() string { return e.Scheme }

func (e *Endpoint) Address() string {
	if e.Scheme == "unix" {
		return e.Path
	}
	return net.JoinHostPort(e.Host, strconv.Itoa(int(e.Port)))
}

func (e *Endpoint) DialTarget() string {
	if e.Scheme == "unix" {
		return "unix://" + e.Path
	}
	return e.Address()
}

func ParseEndpoint(raw string) (*Endpoint, error) {
	scheme, rest, ok := strings.Cut(raw, "://")
	if !ok {
		return nil, errors.Errorf("invalid endpoint %q: missing scheme://", raw)
	}
	switch scheme {
	case "tcp":
		host, port, err := net.SplitHostPort(rest)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid tcp endpoint %q", raw)
		}
		p, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid tcp port in endpoint %q", raw)
		}
		return &Endpoint{Scheme: "tcp", Host: host, Port: uint16(p)}, nil
	case "unix":
		if rest == "" {
			return nil, errors.Errorf("invalid unix endpoint %q: empty path", raw)
		}
		return &Endpoint{Scheme: "unix", Path: rest}, nil
	default:
		return nil, errors.Errorf("unsupported endpoint scheme %q in %q", scheme, raw)
	}
}
