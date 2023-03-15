package scraper

import (
	"context"
	"fmt"
	"github.com/zerlok/monitoring-go"
	"log"
	"time"
)

func WithDefaultLogging() *LoggingOptions {
	return &LoggingOptions{
		Now: func() time.Time {
			return time.Now().UTC()
		},
		Log: func(_ time.Time, operation monitoring.OperationContext, msg string) {
			log.Printf("%s :: %s\n", operation.Name(), msg)
		},
	}
}

type LoggingFactory struct {
	Options *LoggingOptions
}

type NowFunc func() time.Time
type LogFunc func(time time.Time, operation monitoring.OperationContext, msg string)

type LoggingOptions struct {
	Now NowFunc
	Log LogFunc
}

func (f *LoggingFactory) Register(_ context.Context, _ *Config) (ok bool, err error) {
	if f.Options == nil {
		f.Options = WithDefaultLogging()
	}

	return true, nil
}

func (f *LoggingFactory) Create(ctx context.Context, op monitoring.OperationContext) (Scraper, error) {
	l := &logger{ctx, op, f.Options.Now, f.Options.Log}
	l.Start()

	return l, nil
}

func (f *LoggingFactory) Unregister(ctx context.Context) {
}

type logger struct {
	ctx context.Context
	op  monitoring.OperationContext
	now NowFunc
	log LogFunc
}

var _ Scraper = (*logger)(nil)

func (l *logger) Context() context.Context {
	return l.ctx
}

func (l *logger) Operation() monitoring.OperationContext {
	return l.op
}

func (l *logger) AddEvent(name string) {
	l.log(l.now(), l.op, fmt.Sprintf("event occurred: %s", name))
}

func (l *logger) AddError(err error) {
	if err != nil {
		l.log(l.now(), l.op, fmt.Sprintf("error occurred: %s", err.Error()))
	}
}

func (l *logger) Start() {
	l.log(l.now(), l.op, "started")
}

func (l *logger) End() {
	l.op.Finish(nil)

	var msg string
	if err := l.op.Err(); err != nil {
		msg = fmt.Sprintf("failed (duration: %s): %s", l.op.Duration().String(), err.Error())
	} else {
		msg = fmt.Sprintf("succeeded (duration: %s)", l.op.Duration().String())
	}

	l.log(l.now(), l.op, msg)
}

func (l *logger) EndError(err error) {
	l.op.Finish(err)

	l.End()
}
