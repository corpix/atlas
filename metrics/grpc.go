package metrics

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

var (
	grpcCallLabels    = []string{"grpc_type", "grpc_service", "grpc_method"}
	grpcHandledLabels = []string{"grpc_type", "grpc_service", "grpc_method", "grpc_code"}
)

type GRPCMetrics struct {
	serverStarted  *prometheus.CounterVec
	serverHandled  *prometheus.CounterVec
	serverHandling *prometheus.HistogramVec
	clientStarted  *prometheus.CounterVec
	clientHandled  *prometheus.CounterVec
	clientHandling *prometheus.HistogramVec
}

func (m *GRPCMetrics) ServerReporter(ctx context.Context, c interceptors.CallMeta) (interceptors.Reporter, context.Context) {
	m.serverStarted.WithLabelValues(string(c.Typ), c.Service, c.Method).Inc()
	return &grpcReporter{
		handled:  m.serverHandled,
		handling: m.serverHandling,
		meta:     c,
	}, ctx
}

func (m *GRPCMetrics) ClientReporter(ctx context.Context, c interceptors.CallMeta) (interceptors.Reporter, context.Context) {
	m.clientStarted.WithLabelValues(string(c.Typ), c.Service, c.Method).Inc()
	return &grpcReporter{
		handled:  m.clientHandled,
		handling: m.clientHandling,
		meta:     c,
	}, ctx
}

func (m *GRPCMetrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return interceptors.UnaryServerInterceptor(m)
}

func (m *GRPCMetrics) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return interceptors.StreamServerInterceptor(m)
}

func (m *GRPCMetrics) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return interceptors.UnaryClientInterceptor(m)
}

func (m *GRPCMetrics) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return interceptors.StreamClientInterceptor(m)
}

func NewGRPCMetrics(r prometheus.Registerer) *GRPCMetrics {
	m := &GRPCMetrics{
		serverStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_server_started_total",
			Help: "Total number of RPCs started on the server.",
		}, grpcCallLabels),
		serverHandled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_server_handled_total",
			Help: "Total number of RPCs completed on the server, regardless of success or failure.",
		}, grpcHandledLabels),
		serverHandling: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "grpc_server_handling_seconds",
			Help:    "Histogram of response latency of RPCs handled by the server.",
			Buckets: prometheus.DefBuckets,
		}, grpcCallLabels),
		clientStarted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_client_started_total",
			Help: "Total number of RPCs started by the client.",
		}, grpcCallLabels),
		clientHandled: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_client_handled_total",
			Help: "Total number of RPCs completed by the client, regardless of success or failure.",
		}, grpcHandledLabels),
		clientHandling: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "grpc_client_handling_seconds",
			Help:    "Histogram of response latency of RPCs performed by the client.",
			Buckets: prometheus.DefBuckets,
		}, grpcCallLabels),
	}
	r.MustRegister(
		m.serverStarted,
		m.serverHandled,
		m.serverHandling,
		m.clientStarted,
		m.clientHandled,
		m.clientHandling,
	)
	return m
}
