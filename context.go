package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ContextKey struct {
	Name string
}

func (k *ContextKey) String() string {
	return fmt.Sprintf("zerlok/monitoring context key %v", k.Name)
}

var (
	operationContextKey = &ContextKey{"operation-context"}
	now                 = nowUtcImpl
)

type OperationContext interface {
	fmt.Stringer

	Main() OperationContext
	Parent() OperationContext
	Name() string
	StartedAt() *time.Time
	FinishedAt() *time.Time
	Duration() *time.Duration
	Err() error
	Finish(error) *time.Time
}

func MainOperation(name string) (ctx OperationContext) {
	return &operationContextImpl{
		name:      name,
		startedAt: now(),
	}
}

func NestedOperation(parent OperationContext, name string) (ctx OperationContext) {
	if parent != nil {
		ctx = &operationContextImpl{
			parent:    parent,
			name:      name,
			startedAt: now(),
		}
	} else {
		ctx = MainOperation(name)
	}

	return
}

func Operation(ctx context.Context) (op OperationContext) {
	if ctx == nil {
		ctx = context.Background()
	}

	op, _ = ctx.Value(operationContextKey).(OperationContext)

	return
}

func WithOperation(parent context.Context, op OperationContext) (ctx context.Context) {
	if parent == nil {
		parent = context.Background()
	}

	ctx = context.WithValue(parent, operationContextKey, op)

	return
}

type operationContextImpl struct {
	parent     OperationContext
	name       string
	startedAt  time.Time
	finishedAt time.Time
	err        error
	finishOnce sync.Once
}

func (o *operationContextImpl) String() string {
	return fmt.Sprintf("OperationContext{Parent: %v, Name: %v, StartedAt: %v, FinishedAt: %v, Err: %v}", o.parent, o.name, o.startedAt, o.finishedAt, o.err)
}

func (o *operationContextImpl) Main() OperationContext {
	if o.parent != nil {
		return o.parent.Main()
	} else {
		return o
	}
}

func (o *operationContextImpl) Parent() OperationContext {
	return o.parent
}

func (o *operationContextImpl) Name() string {
	return o.name
}

func (o *operationContextImpl) StartedAt() *time.Time {
	return &o.startedAt
}

func (o *operationContextImpl) FinishedAt() *time.Time {
	return &o.finishedAt
}

func (o *operationContextImpl) Duration() *time.Duration {
	d := o.finishedAt.Sub(o.startedAt)

	return &d
}

func (o *operationContextImpl) Err() error {
	return o.err
}

func (o *operationContextImpl) Finish(err error) *time.Time {
	o.finishOnce.Do(func() {
		o.finishedAt = now()
		o.err = err
	})

	return &o.finishedAt
}

type serializableOperationContext struct {
	Main       string `json:"main"`
	MainTs     int64  `json:"mainTs"`
	ParentName string `json:"parent"`
	ParentTs   int64  `json:"parentTs"`
	Name       string `json:"name"`
	Ts         int64  `json:"ts"`
}

func (o *operationContextImpl) MarshalJSON() ([]byte, error) {
	main := o.Main()
	parent := o.Parent()

	return json.Marshal(&serializableOperationContext{
		Main:       main.Name(),
		MainTs:     main.StartedAt().UnixMilli(),
		ParentName: parent.Name(),
		ParentTs:   parent.StartedAt().UnixMilli(),
		Name:       o.name,
		Ts:         o.startedAt.UnixMilli(),
	})
}

func (o *operationContextImpl) UnmarshalJSON(buf []byte) error {
	var ctx serializableOperationContext
	err := json.Unmarshal(buf, &ctx)
	if err != nil {
		return err
	}

	o.parent = &operationContextImpl{
		parent: &operationContextImpl{
			parent:    nil,
			name:      ctx.Main,
			startedAt: time.UnixMilli(ctx.MainTs),
		},
		name:      ctx.ParentName,
		startedAt: time.UnixMilli(ctx.ParentTs),
	}
	o.name = ctx.Name
	o.startedAt = time.UnixMilli(ctx.Ts)

	return nil
}

func EncodeOperation(op OperationContext) ([]byte, error) {
	return json.Marshal(op)
}

func DecodeOperation(buf []byte) (OperationContext, error) {
	var ctx *operationContextImpl
	return ctx, json.Unmarshal(buf, ctx)
}

func nowUtcImpl() time.Time {
	return time.Now().UTC()
}
