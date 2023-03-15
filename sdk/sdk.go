package sdk

import (
	"context"
	"github.com/zerlok/monitoring-go"
	"github.com/zerlok/monitoring-go/scraper"
	"log"
	"os/signal"
	"runtime"
	"strings"
	"sync"
)

type StartupFunc = func(context.Context, *Options) (CleanupFunc, error)
type CleanupFunc = func(context.Context)

type Sdk struct {
	options      *Options
	startups     []StartupFunc
	cleanups     []CleanupFunc
	setupOnce    sync.Once
	shutdownOnce sync.Once
	registry     []scraper.Factory
}

var _ ScrapeBuilder = (*Sdk)(nil)

type ScrapeBuilder interface {
	Group() ScrapeBuilder
	GroupName(string) ScrapeBuilder
	Scrape(context.Context) scraper.Scraper
	ScrapeName(context.Context, string) scraper.Scraper
}

func (s *Sdk) Setup() (err error) {
	return s.SetupContext(context.Background())
}

func (s *Sdk) SetupContext(ctx context.Context) (err error) {
	s.setupOnce.Do(func() {
		cleanups := []CleanupFunc{}
		if s.startups != nil {
			for _, startup := range s.startups {
				cleanup, err := startup(ctx, s.options)
				if err != nil {
					return
				}
				cleanups = append(cleanups, cleanup)
			}
		}

		registered := []scraper.Factory{}
		if s.options.ScraperFactories != nil {
			scraperConfig := &scraper.Config{
				Mux: s.options.ServeMux,
			}

			for _, f := range s.options.ScraperFactories {
				ok, err := f.Register(ctx, scraperConfig)

				if err == nil && ok {
					registered = append(registered, f)
				} else if err != nil {
					return
				}
			}
		}

		s.registry = registered
		s.cleanups = cleanups
	})

	return
}

func (s *Sdk) Group() ScrapeBuilder {
	return s.GroupName(callerFunction(1))
}

func (s *Sdk) GroupName(name string) ScrapeBuilder {
	return &group{s, name}
}

func (s *Sdk) Scrape(ctx context.Context) scraper.Scraper {
	return s.ScrapeName(ctx, callerFunction(1))
}

func (s *Sdk) ScrapeName(ctx context.Context, name string) scraper.Scraper {
	return s.scrape(ctx, monitoring.NestedOperation(monitoring.Operation(ctx), name))
}

func (s *Sdk) ScrapeMain(ctx context.Context) scraper.Scraper {
	return s.ScrapeMainName(ctx, callerFunction(1))
}

func (s *Sdk) ScrapeMainName(ctx context.Context, name string) scraper.Scraper {
	return s.scrape(ctx, monitoring.MainOperation(name))
}

func (s *Sdk) Shutdown() {
	s.ShutdownContext(context.Background())
}

func (s *Sdk) ShutdownContext(ctx context.Context) {
	s.shutdownOnce.Do(func() {
		ctx, cancel := signal.NotifyContext(ctx, s.options.ShutdownSignals...)
		defer cancel()

		if timeout := s.options.ShutdownTimeout; timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		cleanups := s.cleanups
		registered := s.registry
		s.registry = nil
		s.cleanups = nil

		for i := len(registered) - 1; i >= 0; i-- {
			registered[i].Unregister(ctx)
		}

		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	})
}

func (s *Sdk) scrape(ctx context.Context, operation monitoring.OperationContext) scraper.Scraper {
	if len(s.registry) == 0 {
		return scraper.NewEmpty(ctx)
	}

	ctx = monitoring.WithOperation(ctx, operation)

	ss := make([]scraper.Scraper, 0, len(s.registry))
	for _, f := range s.registry {
		s, err := f.Create(ctx, operation)
		if err != nil {
			log.Printf("failed to create scraper for '%v' operation: %v\n", operation.String(), err.Error())
		} else {
			ss = append(ss, s)
		}
	}

	return scraper.NewSequential(ss...)
}

type group struct {
	sdk  *Sdk
	name string
}

var _ ScrapeBuilder = (*group)(nil)

func (g *group) Group() ScrapeBuilder {
	return g.GroupName(callerFunction(1))
}

func (g *group) GroupName(name string) ScrapeBuilder {
	return g.sdk.GroupName(makeGroupName(g.name, name))
}

func (g *group) Scrape(ctx context.Context) scraper.Scraper {
	return g.ScrapeName(ctx, callerFunction(1))
}

func (g *group) ScrapeName(ctx context.Context, name string) scraper.Scraper {
	return g.sdk.scrape(ctx, monitoring.NestedOperation(monitoring.Operation(ctx), makeGroupName(g.name, name)))
}

func callerFunction(skip int) string {
	// https://stackoverflow.com/a/46289376
	pc := make([]uintptr, 15)
	n := runtime.Callers(skip+2, pc)
	frames := runtime.CallersFrames(pc[:n])
	frame, _ := frames.Next()

	return frame.Function
}

func makeGroupName(values ...string) string {
	return strings.Join(values, "::")
}
