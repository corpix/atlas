package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Registry struct {
	*prometheus.Registry
}

func New() *Registry {
	r := &Registry{Registry: prometheus.NewRegistry()}
	r.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewBuildInfoCollector(),
	)
	return r
}
