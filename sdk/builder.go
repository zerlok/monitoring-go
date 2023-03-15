package sdk

import (
	"context"
	"github.com/zerlok/monitoring-go"
	"github.com/zerlok/monitoring-go/scraper"
	"log"
	"net/http"
	"os"
	"syscall"
	"time"
)

var (
	DefaultFactories = &struct {
		PrometheusMetrics scraper.Factory
		OtelTracing       scraper.Factory
		Logging           scraper.Factory
	}{
		PrometheusMetrics: &scraper.PrometheusMetricsFactory{},
		OtelTracing:       &scraper.OtelTracingFactory{},
		Logging:           &scraper.LoggingFactory{},
	}
)

type Options struct {
	ScraperFactories []scraper.Factory
	ServeMux         *http.ServeMux
	AutoServeAddr    string
	ShutdownTimeout  time.Duration
	ShutdownSignals  []os.Signal
}

type Builder struct {
	opts     *Options
	startups []StartupFunc
}

func NewBuilder() *Builder {
	factories := []scraper.Factory{}
	if enabled := monitoring.EnvBool("PROMETHEUS_METRICS_ENABLED", false); enabled {
		factories = append(factories, DefaultFactories.PrometheusMetrics)
	}
	if enabled := monitoring.EnvBool("OTEL_TRACING_ENABLED", false); enabled {
		factories = append(factories, DefaultFactories.OtelTracing)
	}
	if len(factories) == 0 {
		factories = append(factories, DefaultFactories.Logging)
	}

	return &Builder{
		&Options{
			factories,
			http.DefaultServeMux,
			":2112",
			time.Second * 10,
			[]os.Signal{syscall.SIGINT, syscall.SIGTERM},
		},
		[]StartupFunc{},
	}
}

func (b *Builder) With(options *Options, startups []StartupFunc) *Builder {
	if options != nil {
		if options.ScraperFactories != nil {
			b.WithFactories(options.ScraperFactories...)
		} else {
			b.WithFactories()
		}
		if addr := options.AutoServeAddr; addr != "" {
			b.WithDefaultAutoServe(addr)
		} else {
			b.WithoutAutoServe()
		}
		if mux := options.ServeMux; mux != nil {
			b.WithServeMux(mux)
		} else {
			b.WithoutServeMux()
		}
		if timeout := options.ShutdownTimeout; timeout > 0 {
			b.WithShutdownTimeout(timeout)
		} else {
			b.WithoutShutdownTimeout()
		}
		if options.ShutdownSignals != nil {
			b.WithShutdownSignals(options.ShutdownSignals...)
		} else {
			b.WithShutdownSignals()
		}
	}
	b.startups = startups

	return b
}

func (b *Builder) WithFactories(values ...scraper.Factory) *Builder {
	b.opts.ScraperFactories = values

	return b
}

func (b *Builder) AddFactories(values ...scraper.Factory) *Builder {
	b.opts.ScraperFactories = append(b.opts.ScraperFactories, values...)

	return b
}

func (b *Builder) WithDefaultFactories(values ...scraper.Factory) *Builder {
	if len(b.opts.ScraperFactories) == 0 {
		b.opts.ScraperFactories = values
	}

	return b
}

func (b *Builder) WithDefaultAutoServe(addr string) *Builder {
	return b.WithAutoServe(addr, http.DefaultServeMux)
}

func (b *Builder) WithAutoServe(addr string, mux *http.ServeMux) *Builder {
	if addr != "" && mux != nil {
		b.opts.AutoServeAddr = addr
		b.opts.ServeMux = mux
	}

	return b
}

func (b *Builder) WithoutAutoServe() *Builder {
	b.opts.AutoServeAddr = ""

	return b
}

func (b *Builder) WithServeMux(mux *http.ServeMux) *Builder {
	if mux != nil {
		b.opts.ServeMux = mux
	}

	return b
}

func (b *Builder) WithoutServeMux() *Builder {
	b.opts.ServeMux = nil

	return b
}

func (b *Builder) WithShutdownTimeout(value time.Duration) *Builder {
	if value <= 0 {
		b.opts.ShutdownTimeout = value
	}

	return b
}

func (b *Builder) WithoutShutdownTimeout() *Builder {
	b.opts.ShutdownTimeout = 0

	return b
}

func (b *Builder) WithShutdownSignals(values ...os.Signal) *Builder {
	b.opts.ShutdownSignals = values

	return b
}

func (b *Builder) WithStartups(values ...StartupFunc) *Builder {
	b.startups = values

	return b
}

func (b *Builder) AddStartups(values ...StartupFunc) *Builder {
	b.startups = append(b.startups, values...)

	return b
}

func (b *Builder) Build() *Sdk {
	startups := b.startups
	if b.opts.AutoServeAddr != "" {
		startups = append(startups, startListenAndServeHttp)
	}

	return &Sdk{options: b.opts, startups: startups}
}

func NewInMemoryOnly(size uint) (*Sdk, *scraper.InMemoryFactory) {
	factory := scraper.NewInMemoryFactory(size)
	sdk := NewBuilder().WithoutAutoServe().WithFactories(factory).Build()

	return sdk, factory
}

func startListenAndServeHttp(_ context.Context, options *Options) (CleanupFunc, error) {
	srv := &http.Server{Addr: options.AutoServeAddr, Handler: options.ServeMux}

	serve := monitoring.StartTask(func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("fatal http server error: %v\n", err.Error())
		}
	})

	return func(ctx context.Context) {
		defer func() {
			if ctx.Err() != nil {
				_ = srv.Close()
			}
		}()

		if err := srv.Shutdown(ctx); err != nil && err != http.ErrServerClosed {
			log.Printf("fatal http server error: %v\n", err.Error())
		}

		select {
		case <-ctx.Done():
		case <-serve.Done():
		}
	}, nil
}
