package main

import (
	"sync"
)

// Counter keeps track of stuff.
type Counter struct {
	v map[string]int
	sync.Mutex
}

// Inc increments counter for a given key and returns the new value.
func (c *Counter) Inc(key string) int {
	c.Lock()
	defer c.Unlock()
	c.v[key]++

	return c.v[key]
}

// Value returns the counter of a given key.
func (c *Counter) Value(key string) int {
	c.Lock()
	defer c.Unlock()
	return c.v[key]
}

// Dec decrements the counter for the given key and returns the new value.
func (c *Counter) Dec(key string) int {
	c.Lock()
	defer c.Unlock()
	c.v[key]--

	return c.v[key]
}

// NewCounter returns a new goroutine-safe counter.
func NewCounter() *Counter {
	return &Counter{v: make(map[string]int)}
}
