package monitoring

import (
	"sync"
)

type TaskFunc func()

type Task struct {
	fn   TaskFunc
	done chan struct{}
	once sync.Once
}

func NewTask(fn TaskFunc) *Task {
	return &Task{fn: fn, done: make(chan struct{}, 1)}
}

func StartTask(fn TaskFunc) *Task {
	t := NewTask(fn)
	go t.Do()

	return t
}

func (t *Task) Do() {
	t.once.Do(func() {
		defer close(t.done)
		t.fn()
		t.done <- struct{}{}
	})
}

func (t *Task) Done() <-chan struct{} {
	return t.done
}
