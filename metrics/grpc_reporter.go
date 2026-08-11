package metrics

import (
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/status"
)

type grpcReporter struct {
	handled  *prometheus.CounterVec
	handling *prometheus.HistogramVec
	meta     interceptors.CallMeta
}

func (r *grpcReporter) PostCall(err error, duration time.Duration) {
	typ := string(r.meta.Typ)
	r.handled.WithLabelValues(typ, r.meta.Service, r.meta.Method, status.Code(err).String()).Inc()
	r.handling.WithLabelValues(typ, r.meta.Service, r.meta.Method).Observe(duration.Seconds())
}

func (r *grpcReporter) PostMsgSend(any, error, time.Duration) {}

func (r *grpcReporter) PostMsgReceive(any, error, time.Duration) {}
