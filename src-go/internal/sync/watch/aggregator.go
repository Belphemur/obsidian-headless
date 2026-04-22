package watch

import (
	"sync"
	"time"
)

const (
	quiescenceDelay    = 10 * time.Second
	maxSuppressionTime = 10 * time.Minute
)

type pendingEvent struct {
	eventType EventType
	firstSeen time.Time
	lastSeen  time.Time
	timer     *time.Timer
}

type Aggregator struct {
	mu      sync.Mutex
	pending map[string]*pendingEvent
	out     chan<- ScanEvent
}

func NewAggregator(out chan<- ScanEvent) *Aggregator {
	return &Aggregator{pending: map[string]*pendingEvent{}, out: out}
}

func (a *Aggregator) Push(path string, eventType EventType) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if pending, ok := a.pending[path]; ok {
		pending.timer.Stop()
		pending.lastSeen = now
		pending.eventType = eventType
		delay := quiescenceDelay
		if now.Sub(pending.firstSeen) >= maxSuppressionTime {
			delay = 0
		}
		pending.timer = time.AfterFunc(delay, func() { a.emit(path) })
		return
	}
	pending := &pendingEvent{eventType: eventType, firstSeen: now, lastSeen: now}
	pending.timer = time.AfterFunc(quiescenceDelay, func() { a.emit(path) })
	a.pending[path] = pending
}

func (a *Aggregator) Flush() {
	a.mu.Lock()
	paths := make([]string, 0, len(a.pending))
	for path, pending := range a.pending {
		pending.timer.Stop()
		paths = append(paths, path)
	}
	a.mu.Unlock()
	for _, path := range paths {
		a.emit(path)
	}
}

func (a *Aggregator) emit(path string) {
	a.mu.Lock()
	pending, ok := a.pending[path]
	if !ok {
		a.mu.Unlock()
		return
	}
	delete(a.pending, path)
	a.mu.Unlock()
	select {
	case a.out <- ScanEvent{Path: path, Type: pending.eventType, DetectedAt: pending.lastSeen}:
	default:
	}
}
