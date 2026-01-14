package main

import "sync"

type Locked[T any] struct {
	mu  sync.RWMutex
	val T
}

func NewLocked[T any](val T) *Locked[T] {
	return &Locked[T]{
		val: val,
	}
}

func (l *Locked[T]) WithLock(f func(*T)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f(&l.val)
}

func (l *Locked[T]) WithRLock(f func(T)) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	f(l.val)
}
