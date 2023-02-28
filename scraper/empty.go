package scraper

import (
	"context"
	"github.com/zerlok/monitoring-go"
)

type empty struct {
	ctx context.Context
}

func NewEmpty(ctx context.Context) Scraper {
	if ctx == nil {
		ctx = context.Background()
	}

	return &empty{ctx}
}

func (e *empty) Context() context.Context { return e.ctx }

func (e *empty) Operation() monitoring.OperationContext { return nil }

func (e *empty) AddEvent(_ string) {}

func (e *empty) AddError(_ error) {}

func (e *empty) End() {}

func (e *empty) EndError(_ error) {}
