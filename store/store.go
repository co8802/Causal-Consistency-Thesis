// Package store is the fake database: a few datacenters, each split into
// partitions. A write goes to one place, then gets copied to the other datacenters.
package store

import (
	"hash/fnv"

	"causal-thesis/simnet"
)

// WriteID names one write. The CLIENT makes it up (like "c1-3"), so a later
// write can point at an earlier one before the earlier one has even returned.
type WriteID string

// Dep means "this write must not show up anywhere until write ID of key Key has."
type Dep struct {
	Key string
	ID  WriteID
}

// Write is one put: a key, a value, its own id, and what it depends on.
type Write struct {
	Key  string
	Val  string
	ID   WriteID
	Deps []Dep
}

// Config holds the knobs of the fake system.
type Config struct {
	DCs   int     // number of datacenters
	Parts int     // partitions (servers) per datacenter
	Local float64 // average one way delay inside a datacenter, in ms
	WAN   float64 // average one way delay between datacenters, in ms
	// LocalDepCheck: if true, a server also waits for a write's dependencies
	// before committing it in the datacenter the client wrote to.
	// Remote datacenters always check. This switch is for our ablation.
	LocalDepCheck bool
}

// Node is one server: datacenter dc, partition part.
type Node struct {
	c       *Cluster
	dc      int
	part    int
	data    map[string]string    // key -> latest value committed here
	applied map[WriteID]bool     // which writes are committed here
	waiters map[WriteID][]func() // things waiting for a write to commit here
}

// Cluster is all the nodes plus the network between them.
type Cluster struct {
	Sim   *simnet.Sim
	cfg   Config
	nodes [][]*Node
	// OnCommit is called every time any node commits a write.
	// The checker uses it to watch what happens.
	OnCommit func(dc, part int, w Write)
}

// NewCluster builds DCs x Parts empty nodes.
func NewCluster(sim *simnet.Sim, cfg Config) *Cluster {
	c := &Cluster{Sim: sim, cfg: cfg}
	for d := 0; d < cfg.DCs; d++ {
		var row []*Node
		for p := 0; p < cfg.Parts; p++ {
			row = append(row, &Node{
				c: c, dc: d, part: p,
				data:    map[string]string{},
				applied: map[WriteID]bool{},
				waiters: map[WriteID][]func(){},
			})
		}
		c.nodes = append(c.nodes, row)
	}
	return c
}

// PartOf says which partition owns a key (a hash, same answer every time).
func (c *Cluster) PartOf(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()) % c.cfg.Parts
}

// delay turns an average delay into a slightly random one (plus or minus 20%).
func (c *Cluster) delay(avg float64) float64 {
	return c.Sim.Uniform(avg*0.8, avg*1.2)
}

// Has says whether write d is committed in datacenter dc.
func (c *Cluster) Has(dc int, d Dep) bool {
	return c.nodes[dc][c.PartOf(d.Key)].applied[d.ID]
}

// Value reads the latest committed value of a key in a datacenter.
func (c *Cluster) Value(dc int, key string) (string, bool) {
	v, ok := c.nodes[dc][c.PartOf(key)].data[key]
	return v, ok
}

// Put sends write w to datacenter dc. done() is called when the client
// would get the reply: after the write committed there and the reply came back.
func (c *Cluster) Put(dc int, w Write, done func()) {
	part := c.PartOf(w.Key)
	n := c.nodes[dc][part]
	// client -> node
	c.Sim.After(c.delay(c.cfg.Local), func() {
		n.accept(w, c.cfg.LocalDepCheck, func() {
			// copy the write to every other datacenter
			for d := 0; d < c.cfg.DCs; d++ {
				if d == dc {
					continue
				}
				remote := c.nodes[d][part]
				c.Sim.After(c.delay(c.cfg.WAN), func() {
					remote.accept(w, true, func() {})
				})
			}
			// node -> client
			c.Sim.After(c.delay(c.cfg.Local), done)
		})
	})
}

// accept commits w here, but first (if check is true) waits for its deps.
func (n *Node) accept(w Write, check bool, then func()) {
	if !check {
		n.commit(w)
		then()
		return
	}
	n.depCheck(w, func() {
		n.commit(w)
		then()
	})
}

// depCheck asks the owner of each dependency "do you have it yet?"
// and calls then() once every answer is yes.
func (n *Node) depCheck(w Write, then func()) {
	left := len(w.Deps)
	if left == 0 {
		then()
		return
	}
	oneDone := func() {
		left--
		if left == 0 {
			then()
		}
	}
	for _, d := range w.Deps {
		d := d
		owner := n.c.nodes[n.dc][n.c.PartOf(d.Key)]
		if owner == n {
			// the dependency lives on this same server, no message needed
			owner.whenApplied(d.ID, oneDone)
			continue
		}
		// ask the other server, wait for it to have the write, hear back
		n.c.Sim.After(n.c.delay(n.c.cfg.Local), func() {
			owner.whenApplied(d.ID, func() {
				n.c.Sim.After(n.c.delay(n.c.cfg.Local), oneDone)
			})
		})
	}
}

// whenApplied runs fn right now if write id is committed here, otherwise
// as soon as it gets committed.
func (n *Node) whenApplied(id WriteID, fn func()) {
	if n.applied[id] {
		fn()
		return
	}
	n.waiters[id] = append(n.waiters[id], fn)
}

// commit makes the write visible on this node.
func (n *Node) commit(w Write) {
	if n.applied[w.ID] {
		return
	}
	n.data[w.Key] = w.Val
	n.applied[w.ID] = true
	if n.c.OnCommit != nil {
		n.c.OnCommit(n.dc, n.part, w)
	}
	ws := n.waiters[w.ID]
	delete(n.waiters, w.ID)
	for _, fn := range ws {
		fn()
	}
}
