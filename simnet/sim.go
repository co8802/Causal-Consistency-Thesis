// Package simnet is a tiny fake clock plus event queue.
// Nothing here uses real time, so the same seed always gives the same run.
package simnet

import (
	"container/heap"
	"math/rand"
)

// event is one thing that will happen at a future (fake) time.
type event struct {
	at  float64 // time in milliseconds
	seq int     // tie breaker so equal times run in the order they were scheduled
	fn  func()
}

// queue is a min-heap of events, earliest first.
type queue []*event

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].seq < q[j].seq
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(*event)) }
func (q *queue) Pop() any {
	old := *q
	n := len(old)
	e := old[n-1]
	*q = old[:n-1]
	return e
}

// Sim is the whole fake world: a clock, a queue, and a seeded random source.
type Sim struct {
	now float64
	seq int
	q   queue
	rng *rand.Rand
}

// New makes a simulator. Same seed means same run.
func New(seed int64) *Sim {
	return &Sim{rng: rand.New(rand.NewSource(seed))}
}

// Now returns the current fake time in ms.
func (s *Sim) Now() float64 { return s.now }

// After schedules fn to run delay ms from now.
func (s *Sim) After(delay float64, fn func()) {
	s.seq++
	heap.Push(&s.q, &event{at: s.now + delay, seq: s.seq, fn: fn})
}

// Run keeps popping the earliest event and running it until none are left.
func (s *Sim) Run() {
	for s.q.Len() > 0 {
		e := heap.Pop(&s.q).(*event)
		s.now = e.at
		e.fn()
	}
}

// Uniform gives a random number between lo and hi from the seeded source.
func (s *Sim) Uniform(lo, hi float64) float64 {
	return lo + s.rng.Float64()*(hi-lo)
}
