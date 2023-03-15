package scraper

import (
	"context"
	"fmt"
	"github.com/zerlok/monitoring-go"
	"sync"
)

func NewInMemoryFactory(size uint) *InMemoryFactory {
	return &InMemoryFactory{ended: make(chan *InMemory, size)}
}

type InMemoryFactory struct {
	setupOnce    sync.Once
	shutdownOnce sync.Once
	ended        chan *InMemory
}

func (i *InMemoryFactory) Register(_ context.Context, _ *Config) (ok bool, err error) {
	i.setupOnce.Do(func() {
		ok = i.ended != nil
	})

	return
}

func (i *InMemoryFactory) Create(ctx context.Context, op monitoring.OperationContext) (Scraper, error) {
	if i.ended == nil {
		return nil, fmt.Errorf("in memory scraping is not initialized")
	}

	return &InMemory{ctx: ctx, op: op, Events: make([]string, 0), Errs: make([]error, 0), ended: i.ended}, nil
}

func (i *InMemoryFactory) Unregister(_ context.Context) {
	i.shutdownOnce.Do(func() {
		ended := i.ended
		i.ended = nil

		close(ended)
	})
}

func (i *InMemoryFactory) Ended() <-chan *InMemory {
	return i.ended
}

func (i *InMemoryFactory) GatherEndedScrapes(ctx context.Context, dest *[]*InMemory) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func(ctx context.Context) {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-i.Ended():
				if !ok {
					return
				}
				*dest = append(*dest, v)
			}
		}
	}(ctx)

	return cancel
}

func (i *InMemoryFactory) GatherEndedOperations(ctx context.Context, dest *[]monitoring.OperationContext) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func(ctx context.Context) {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-i.Ended():
				if !ok {
					return
				}
				*dest = append(*dest, v.Operation())
			}
		}
	}(ctx)

	return cancel
}

type InMemory struct {
	ctx    context.Context
	op     monitoring.OperationContext
	mux    sync.Mutex
	Events []string
	Errs   []error
	ended  chan<- *InMemory
}

var _ Scraper = (*InMemory)(nil)

func (i *InMemory) Context() context.Context {
	return i.ctx
}

func (i *InMemory) Operation() monitoring.OperationContext {
	return i.op
}

func (i *InMemory) AddEvent(name string) {
	i.mux.Lock()
	defer i.mux.Unlock()

	i.Events = append(i.Events, name)
}

func (i *InMemory) AddError(err error) {
	i.mux.Lock()
	defer i.mux.Unlock()

	i.Errs = append(i.Errs, err)
}

func (i *InMemory) End() {
	i.op.Finish(nil)

	i.AddError(i.op.Err())
	i.ended <- i
}

func (i *InMemory) EndError(err error) {
	i.op.Finish(err)

	i.End()
}
