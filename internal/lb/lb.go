package lb

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
)

// Strategy represents the load balancing strategy.
type Strategy string

const (
	Priority     Strategy = "priority"
	RoundRobin   Strategy = "round-robin"
	LeastLatency Strategy = "least-latency"
	Weighted     Strategy = "weighted"
	Random       Strategy = "random"
)

// Connection represents a load-balanced upstream connection.
type Connection struct {
	ID       string
	Priority int
	Weight   int
	Active   bool
	Latency  float64 // EWMA latency in milliseconds
}

// latencyTracker tracks exponential weighted moving average latency.
type latencyTracker struct {
	ewma  float64
	alpha float64 // smoothing factor, default 0.3
	count int
	mu    sync.Mutex
}

// LoadBalancer distributes requests across connections using a configurable strategy.
type LoadBalancer struct {
	strategy  Strategy
	mu        sync.RWMutex
	rrCounter uint64
	latencies map[string]*latencyTracker
	conns     []Connection
}

// New creates a new LoadBalancer with the given strategy and connections.
func New(strategy Strategy, conns []Connection) *LoadBalancer {
	lb := &LoadBalancer{
		strategy:  strategy,
		latencies: make(map[string]*latencyTracker),
		conns:     make([]Connection, len(conns)),
	}
	copy(lb.conns, conns)
	for i := range lb.conns {
		if lb.conns[i].Weight <= 0 {
			lb.conns[i].Weight = 1
		}
		lb.latencies[lb.conns[i].ID] = &latencyTracker{
			ewma:  0,
			alpha: 0.3,
		}
	}
	return lb
}

// Pick selects a connection based on the current strategy.
func (lb *LoadBalancer) Pick() (*Connection, error) {
	lb.mu.RLock()
	strategy := lb.strategy
	lb.mu.RUnlock()

	switch strategy {
	case Priority:
		return lb.pickPriority()
	case RoundRobin:
		return lb.pickRoundRobin()
	case LeastLatency:
		return lb.pickLeastLatency()
	case Weighted:
		return lb.pickWeighted()
	case Random:
		return lb.pickRandom()
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}
}

// RecordLatency updates the EWMA latency for a connection.
func (lb *LoadBalancer) RecordLatency(connID string, ms float64) {
	lb.mu.RLock()
	tracker, ok := lb.latencies[connID]
	lb.mu.RUnlock()
	if !ok {
		return
	}
	tracker.mu.Lock()
	tracker.count++
	if tracker.count == 1 {
		tracker.ewma = ms
	} else {
		tracker.ewma = tracker.alpha*ms + (1-tracker.alpha)*tracker.ewma
	}
	tracker.mu.Unlock()

	// Also update the connection's Latency field for external reads.
	lb.mu.Lock()
	for i := range lb.conns {
		if lb.conns[i].ID == connID {
			lb.conns[i].Latency = tracker.ewma
			break
		}
	}
	lb.mu.Unlock()
}

// UpdateConnections refreshes the connection list, preserving latency data for existing IDs.
func (lb *LoadBalancer) UpdateConnections(conns []Connection) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.conns = make([]Connection, len(conns))
	copy(lb.conns, conns)
	for i := range lb.conns {
		if lb.conns[i].Weight <= 0 {
			lb.conns[i].Weight = 1
		}
		// Preserve existing latency data or create new tracker
		if _, ok := lb.latencies[lb.conns[i].ID]; !ok {
			lb.latencies[lb.conns[i].ID] = &latencyTracker{
				ewma:  0,
				alpha: 0.3,
			}
		}
		// Sync latency to connection
		lb.conns[i].Latency = lb.latencies[lb.conns[i].ID].ewma
	}
}

// SetStrategy hot-swaps the load balancing strategy.
func (lb *LoadBalancer) SetStrategy(s Strategy) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.strategy = s
}

// ActiveConnections returns all currently active connections.
func (lb *LoadBalancer) activeConns() []Connection {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	var active []Connection
	for _, c := range lb.conns {
		if c.Active {
			active = append(active, c)
		}
	}
	return active
}

func (lb *LoadBalancer) pickPriority() (*Connection, error) {
	active := lb.activeConns()
	if len(active) == 0 {
		return nil, fmt.Errorf("no active connections available")
	}
	best := active[0]
	for _, c := range active[1:] {
		if c.Priority > best.Priority {
			best = c
		}
	}
	return &best, nil
}

func (lb *LoadBalancer) pickRoundRobin() (*Connection, error) {
	active := lb.activeConns()
	if len(active) == 0 {
		return nil, fmt.Errorf("no active connections available")
	}
	// atomic.AddUint64 returns the value AFTER increment, so subtract 1
	// to get a 0-based counter: first call returns 0, second returns 1, etc.
	idx := (atomic.AddUint64(&lb.rrCounter, 1) - 1) % uint64(len(active))
	// Make a copy to return a stable pointer
	result := active[idx]
	return &result, nil
}

func (lb *LoadBalancer) pickLeastLatency() (*Connection, error) {
	active := lb.activeConns()
	if len(active) == 0 {
		return nil, fmt.Errorf("no active connections available")
	}
	// Read latest latencies
	lb.mu.RLock()
	best := active[0]
	bestLatency := math.MaxFloat64
	for _, c := range active {
		lat := lb.latencies[c.ID].ewma
		// If no latency data yet, treat as 0 (prefer untested connections)
		if lat < bestLatency {
			bestLatency = lat
			best = c
		}
	}
	lb.mu.RUnlock()
	return &best, nil
}

func (lb *LoadBalancer) pickWeighted() (*Connection, error) {
	active := lb.activeConns()
	if len(active) == 0 {
		return nil, fmt.Errorf("no active connections available")
	}
	totalWeight := 0
	for _, c := range active {
		totalWeight += c.Weight
	}
	if totalWeight == 0 {
		return nil, fmt.Errorf("total weight is zero")
	}
	// Choose a random target in [0, totalWeight)
	n, err := cryptoRandInt(totalWeight)
	if err != nil {
		// Fallback: use first active
		return &active[0], nil
	}
	cumulative := 0
	for _, c := range active {
		cumulative += c.Weight
		if n < cumulative {
			return &c, nil
		}
	}
	// Fallback: last connection
	return &active[len(active)-1], nil
}

func (lb *LoadBalancer) pickRandom() (*Connection, error) {
	active := lb.activeConns()
	if len(active) == 0 {
		return nil, fmt.Errorf("no active connections available")
	}
	n, err := cryptoRandInt(len(active))
	if err != nil {
		return &active[0], nil
	}
	return &active[n], nil
}

// cryptoRandInt returns a random integer in [0, max).
// Uses crypto/rand for unbiased randomness.
func cryptoRandInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive")
	}
	// Small max: use math/big
	if max <= 1<<31 {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, err
		}
		n := binary.BigEndian.Uint64(buf[:])
		return int(n % uint64(max)), nil
	}
	// Large max
	bigMax := big.NewInt(int64(max))
	bigN, err := rand.Int(rand.Reader, bigMax)
	if err != nil {
		return 0, err
	}
	return int(bigN.Int64()), nil
}
