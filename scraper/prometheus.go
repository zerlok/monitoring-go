package scraper

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zerlok/monitoring-go"
	"log"
	"net/http"
	"reflect"
	"sync"
)

// TODO: try prometheus exporter from OTEL: https://opentelemetry.io/docs/instrumentation/go/exporters/#prometheus-exporter

const (
	emptyLabelValue = ""
	globalLabel     = "global"
	parentLabel     = "parent"
	nameLabel       = "name"
)

var (
	operationBaseLabelNames        = []string{globalLabel, parentLabel, nameLabel}
	defaultPrometheusMetricOptions = &PrometheusMetricsOptions{
		ServeMux:   http.DefaultServeMux,
		Endpoint:   "/metrics",
		Handler:    promhttp.Handler(),
		Registerer: prometheus.DefaultRegisterer,
		Namespace:  "ops",
	}
)

type PrometheusMetricsFactory struct {
	Options        *PrometheusMetricsOptions
	registerOnce   sync.Once
	unregisterOnce sync.Once
	active         *prometheus.GaugeVec
	totalEvents    *prometheus.CounterVec
	totalProcessed *prometheus.CounterVec
	totalDurations *prometheus.CounterVec
	totalErrors    *prometheus.CounterVec
}

type PrometheusMetricsOptions struct {
	ServeMux   *http.ServeMux
	Endpoint   string
	Handler    http.Handler
	Registerer prometheus.Registerer
	Namespace  string
}

func (f *PrometheusMetricsFactory) Register(_ context.Context, config *Config) (ok bool, err error) {
	f.registerOnce.Do(func() {
		if f.Options == nil {
			f.Options = defaultPrometheusMetricOptions
		}

		config.Handle(f.Options.Endpoint, f.Options.Handler)

		factory := promauto.With(f.Options.Registerer)

		f.active = factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: f.Options.Namespace,
			Name:      "active",
			Help:      "Amount of active operations (not processed yet)",
		}, operationBaseLabelNames)
		f.totalProcessed = factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: f.Options.Namespace,
			Name:      "processed_total",
			Help:      "Total count of processed operations",
		}, operationBaseLabelNames)
		f.totalDurations = factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: f.Options.Namespace,
			Name:      "processed_duration_seconds_total",
			Help:      "Total duration sum of processed operations (in seconds)",
		}, operationBaseLabelNames)
		f.totalErrors = factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: f.Options.Namespace,
			Name:      "errors_total",
			Help:      "Total count of occurred errors in operations",
		}, extended(operationBaseLabelNames, "type"))
		f.totalEvents = factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: f.Options.Namespace,
			Name:      "events_total",
			Help:      "Total count of occurred events in operations (except ",
		}, extended(operationBaseLabelNames, "value"))

		ok = true
	})

	return
}

func (f *PrometheusMetricsFactory) Create(ctx context.Context, op monitoring.OperationContext) (s Scraper, err error) {
	labels := operationLabels(op)

	active, err := f.active.GetMetricWith(labels)
	if err != nil {
		return
	}

	totalProcessed, err := f.totalProcessed.GetMetricWith(labels)
	if err != nil {
		return
	}

	totalDurations, err := f.totalDurations.GetMetricWith(labels)
	if err != nil {
		return
	}

	totalErrors, err := f.totalErrors.CurryWith(labels)
	if err != nil {
		return
	}

	totalEvents, err := f.totalEvents.CurryWith(labels)
	if err != nil {
		return
	}

	defer active.Inc()

	s = &prometheusMetric{
		ctx,
		op,
		active,
		totalProcessed,
		totalDurations,
		totalErrors,
		totalEvents,
	}

	return
}

func (f *PrometheusMetricsFactory) Unregister(_ context.Context) {}

type prometheusMetric struct {
	ctx            context.Context
	op             monitoring.OperationContext
	active         prometheus.Gauge
	totalProcessed prometheus.Counter
	totalDurations prometheus.Counter
	totalErrors    *prometheus.CounterVec
	totalEvents    *prometheus.CounterVec
}

func (m *prometheusMetric) Context() context.Context {
	return m.ctx
}

func (m *prometheusMetric) Operation() monitoring.OperationContext {
	return m.op
}

func (m *prometheusMetric) AddEvent(name string) {
	counter, err := m.totalEvents.GetMetricWithLabelValues(name)
	if err != nil {
		log.Printf("failed to update event counter: %v\n", err.Error())
		return
	}

	counter.Inc()
}

func (m *prometheusMetric) AddError(origErr error) {
	if origErr == nil {
		return
	}

	counter, err := m.totalErrors.GetMetricWithLabelValues(reflect.TypeOf(origErr).String())
	if err != nil {
		log.Printf("failed to update error counter (original err: %v): %v\n", origErr.Error(), err.Error())
		return
	}

	counter.Inc()
}

func (m *prometheusMetric) End() {
	m.op.Finish(nil)

	m.AddError(m.op.Err())
	m.active.Dec()
	m.totalProcessed.Inc()
	m.totalDurations.Add(m.op.Duration().Seconds())
}

func (m *prometheusMetric) EndError(err error) {
	m.op.Finish(err)

	m.End()
}

func operationLabels(op monitoring.OperationContext) map[string]string {
	labels := map[string]string{
		globalLabel: emptyLabelValue,
		parentLabel: emptyLabelValue,
		nameLabel:   emptyLabelValue,
	}

	if op != nil {
		if main := op.Main(); main != nil {
			labels[globalLabel] = main.Name()
		}
		if parent := op.Parent(); parent != nil {
			labels[parentLabel] = parent.Name()
		}
		labels[nameLabel] = op.Name()
	}

	return labels
}

func extended[T any](xs []T, other ...T) []T {
	return append(append(make([]T, 0, len(xs)+len(other)), xs...), other...)
}
