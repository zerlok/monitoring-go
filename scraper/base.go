package scraper

import (
	"context"
	"github.com/zerlok/monitoring-go"
)

type Scraper interface {
	Context() context.Context
	Operation() monitoring.OperationContext
	AddEvent(string)
	AddError(error)
	End()
	EndError(error)
}

type Factory interface {
	Register(context.Context, *Config) (bool, error)
	Create(context.Context, monitoring.OperationContext) (Scraper, error)
	Unregister(context.Context)
}
