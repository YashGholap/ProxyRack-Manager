package pool

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Manager struct {
	mu sync.RWMutex

	maxThreads int
	sem        chan struct{}

	leases map[string]*atomic.Int64

	inFlight atomic.Int64

	waiting atomic.Int64

	maxQueueDepth int
}

func NewManager(maxThreads, maxQueueDepth int) *Manager {
	m := &Manager{
		maxThreads:    maxThreads,
		sem:           make(chan struct{}, maxThreads),
		leases:        make(map[string]*atomic.Int64),
		maxQueueDepth: maxQueueDepth,
	}
	return m
}

func (m *Manager) Acquire(service string) (release func(), err error) {
	currentWaiting := m.waiting.Load()
	if int(currentWaiting) >= m.maxQueueDepth {
		return nil, fmt.Errorf("queue full: %d requests already waiting", currentWaiting)
	}

	m.waiting.Add(1)

	m.sem <- struct{}{}

	m.waiting.Add(-1)
	m.inFlight.Add(1)

	counter := m.getOrCreateLease(service)
	counter.Add(1)

	released := false
	release = func() {
		if released {
			return
		}
		released = true
		counter.Add(-1)
		m.inFlight.Add(-1)
		<-m.sem
	}
	return release, nil
}

func (m *Manager) TryAcquire(service string) (release func(), err error) {
	select {
	case m.sem <- struct{}{}:
		m.inFlight.Add(1)
		counter := m.getOrCreateLease(service)
		counter.Add(1)

		released := false
		release = func() {
			if released {
				return
			}

			released = true
			counter.Add(-1)
			m.inFlight.Add(-1)
			<-m.sem
		}
		return release, nil

	default:
		return nil, fmt.Errorf("no slots available")
	}
}

type Status struct {
	Max       int            `json:"max"`
	InUse     int            `json:"in_use"`
	Available int            `json:"available"`
	Waiting   int            `json:"waiting"`
	Leases    map[string]int `json:"leases"`
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	inUse := int(m.inFlight.Load())
	leases := make(map[string]int, len(m.leases))
	for svc, counter := range m.leases {
		v := int(counter.Load())
		if v > 0 {
			leases[svc] = v
		}
	}

	return Status{
		Max:       m.maxThreads,
		InUse:     inUse,
		Available: m.maxThreads - inUse,
		Waiting:   int(m.waiting.Load()),
		Leases:    leases,
	}
}

func (m *Manager) UpdateMaxThreads(newMax int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if newMax <= 0 || newMax == m.maxThreads {
		return
	}

	newSem := make(chan struct{}, newMax)

	inFlight := int(m.inFlight.Load())
	for i := 0; i < inFlight && i < newMax; i++ {
		newSem <- struct{}{}
	}

	m.sem = newSem
	m.maxThreads = newMax
}

func (m *Manager) getOrCreateLease(service string) *atomic.Int64 {
	m.mu.RLock()
	counter, exists := m.leases[service]
	m.mu.RUnlock()

	if exists {
		return counter
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if counter, exists = m.leases[service]; exists {
		return counter
	}

	counter = &atomic.Int64{}
	m.leases[service] = counter
	return counter
}
