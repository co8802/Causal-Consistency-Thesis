package simnet

import "testing"

// events must run in time order, not in the order they were scheduled
func TestOrder(t *testing.T) {
	s := New(1)
	var got []int
	s.After(5, func() { got = append(got, 2) })
	s.After(1, func() { got = append(got, 1) })
	s.After(9, func() { got = append(got, 3) })
	s.Run()
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("wrong order: %v", got)
	}
	if s.Now() != 9 {
		t.Fatalf("clock should end at 9, got %v", s.Now())
	}
}

// an event can schedule another event
func TestChain(t *testing.T) {
	s := New(1)
	done := false
	s.After(2, func() { s.After(3, func() { done = true }) })
	s.Run()
	if !done || s.Now() != 5 {
		t.Fatalf("chain failed, done=%v now=%v", done, s.Now())
	}
}

// same seed gives the same numbers
func TestSeed(t *testing.T) {
	a, b := New(7), New(7)
	for i := 0; i < 5; i++ {
		if a.Uniform(0, 1) != b.Uniform(0, 1) {
			t.Fatal("same seed gave different numbers")
		}
	}
}
