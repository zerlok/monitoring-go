package scraper

import (
	"context"
	"github.com/zerlok/monitoring-go"
)

func NewSequential(seq ...Scraper) Scraper {
	return &sequential{seq}
}

type sequential struct {
	seq []Scraper
}

func (s *sequential) Context() (ctx context.Context) {
	ls := s.last()
	if ls != nil {
		ctx = ls.Context()
	}

	return
}

func (s *sequential) Operation() (op monitoring.OperationContext) {
	ls := s.last()
	if ls != nil {
		op = ls.Operation()
	}

	return
}

func (s *sequential) AddEvent(name string) {
	for _, inner := range s.seq {
		inner.AddEvent(name)
	}
}

func (s *sequential) AddError(err error) {
	for i := len(s.seq) - 1; i >= 0; i-- {
		s.seq[i].AddError(err)
	}
}

func (s *sequential) End() {
	if op := s.Operation(); op != nil {
		op.Finish(nil)
	}

	for i := len(s.seq) - 1; i >= 0; i-- {
		s.seq[i].End()
	}
}

func (s *sequential) EndError(err error) {
	if op := s.Operation(); op != nil {
		op.Finish(err)
	}

	s.End()
}

func (s *sequential) last() Scraper {
	if len(s.seq) > 0 {
		return s.seq[len(s.seq)-1]
	} else {
		return nil
	}
}
