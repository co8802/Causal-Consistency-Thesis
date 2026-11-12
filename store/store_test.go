package store

import (
	"fmt"
	"testing"

	"causal-thesis/simnet"
)

// keyOn finds a key that lives on the given partition.
func keyOn(c *Cluster, part int, tag string) string {
	for i := 0; ; i++ {
		k := fmt.Sprintf("%s%d", tag, i)
		if c.PartOf(k) == part {
			return k
		}
	}
}

func cfg(local bool) Config {
	return Config{DCs: 3, Parts: 2, Local: 0.2, WAN: 5, LocalDepCheck: local}
}

// one write should end up committed on every datacenter
func TestPutReachesEveryDC(t *testing.T) {
	sim := simnet.New(1)
	c := NewCluster(sim, cfg(true))
	w := Write{Key: "x", Val: "1", ID: "c1-1"}
	replied := false
	c.Put(0, w, func() { replied = true })
	sim.Run()
	if !replied {
		t.Fatal("client never got a reply")
	}
	for dc := 0; dc < 3; dc++ {
		if v, ok := c.Value(dc, "x"); !ok || v != "1" {
			t.Fatalf("dc %d missing the write", dc)
		}
	}
}

// a write with a dependency must wait for it, even if it arrives first
func TestWaitsForDependency(t *testing.T) {
	sim := simnet.New(1)
	c := NewCluster(sim, cfg(true))
	a := Write{Key: keyOn(c, 0, "a"), Val: "A", ID: "c1-1"}
	b := Write{Key: keyOn(c, 1, "b"), Val: "B", ID: "c1-2", Deps: []Dep{{a.Key, a.ID}}}
	// give B to the node first, long before A, directly in dc 1
	var order []WriteID
	c.OnCommit = func(dc, part int, w Write) {
		if dc == 1 {
			order = append(order, w.ID)
		}
	}
	n := c.nodes[1][1]
	n.accept(b, true, func() {})
	sim.After(50, func() { c.nodes[1][0].accept(a, true, func() {}) })
	sim.Run()
	if len(order) != 2 || order[0] != "c1-1" || order[1] != "c1-2" {
		t.Fatalf("B committed before A: %v", order)
	}
}

// runChain sends a chain of writes WITHOUT waiting for replies (each depends
// on the one before) and counts commits that happened before their deps.
func runChain(seed int64, local bool) int {
	sim := simnet.New(seed)
	c := NewCluster(sim, cfg(local))
	bad := 0
	c.OnCommit = func(dc, part int, w Write) {
		for _, d := range w.Deps {
			if !c.Has(dc, d) {
				bad++
			}
		}
	}
	var prev *Write
	for i := 1; i <= 6; i++ {
		w := Write{
			Key: keyOn(c, i%2, fmt.Sprintf("k%d-", i)),
			Val: "v",
			ID:  WriteID(fmt.Sprintf("c1-%d", i)),
		}
		if prev != nil {
			w.Deps = []Dep{{prev.Key, prev.ID}}
		}
		c.Put(0, w, func() {})
		ww := w
		prev = &ww
	}
	sim.Run()
	return bad
}

// with the local check on, no seed should ever break causal order
func TestChainSafeWithLocalCheck(t *testing.T) {
	for seed := int64(1); seed <= 500; seed++ {
		if bad := runChain(seed, true); bad != 0 {
			t.Fatalf("seed %d: %d writes committed before their deps", seed, bad)
		}
	}
}

// with the local check off, overlapping writes should sometimes break it.
// If this test fails, our local check isn't actually doing anything.
func TestChainBreaksWithoutLocalCheck(t *testing.T) {
	broken := 0
	for seed := int64(1); seed <= 500; seed++ {
		if runChain(seed, false) > 0 {
			broken++
		}
	}
	t.Logf("seeds that broke without the local check: %d of 500", broken)
	if broken == 0 {
		t.Fatal("expected some violations without the local check")
	}
}
