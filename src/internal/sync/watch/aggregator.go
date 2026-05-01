package watch

import (
	"context"
	"sync"
	"time"
)

var (
	quiescenceDelay    = 2 * time.Second
	maxSuppressionTime = 10 * time.Minute
)

type pendingEvent struct {
	eventType EventType
	firstSeen time.Time
	lastSeen  time.Time
	timer     *time.Timer
	token     uint64
	oldPath   string // only set for rename events
}

type Aggregator struct {
	mu      sync.Mutex
	pending map[string]*pendingEvent
	out     chan<- ScanEvent
	closed  bool
}

func NewAggregator(out chan<- ScanEvent) *Aggregator {
	return &Aggregator{pending: map[string]*pendingEvent{}, out: out}
}

func (a *Aggregator) Push(path string, eventType EventType) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	now := time.Now()
	if pending, ok := a.pending[path]; ok {
		pending.timer.Stop()
		pending.lastSeen = now
		pending.eventType = eventType
		pending.token++
		token := pending.token
		delay := quiescenceDelay
		if now.Sub(pending.firstSeen) >= maxSuppressionTime {
			delay = 0
		}
		pending.timer = time.AfterFunc(delay, func() { a.emit(path, pending, token) })
		return
	}
	pending := &pendingEvent{eventType: eventType, firstSeen: now, lastSeen: now, token: 1}
	pending.timer = time.AfterFunc(quiescenceDelay, func() { a.emit(path, pending, pending.token) })
	a.pending[path] = pending
}

func (a *Aggregator) PushRename(path, oldPath string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	now := time.Now()
	if pending, ok := a.pending[path]; ok {
		pending.timer.Stop()
		pending.lastSeen = now
		pending.eventType = EventRename
		pending.oldPath = oldPath
		pending.token++
		token := pending.token
		delay := quiescenceDelay
		if now.Sub(pending.firstSeen) >= maxSuppressionTime {
			delay = 0
		}
		pending.timer = time.AfterFunc(delay, func() { a.emit(path, pending, token) })
		return
	}
	pending := &pendingEvent{eventType: EventRename, firstSeen: now, lastSeen: now, token: 1, oldPath: oldPath}
	pending.timer = time.AfterFunc(quiescenceDelay, func() { a.emit(path, pending, pending.token) })
	a.pending[path] = pending
}

func (a *Aggregator) Shutdown(ctx context.Context) bool {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return true
	}
	a.closed = true
	events := make([]ScanEvent, 0, len(a.pending))
	for path, pending := range a.pending {
		pending.timer.Stop()
		events = append(events, ScanEvent{Path: path, Type: pending.eventType, DetectedAt: pending.lastSeen, OldPath: pending.oldPath})
	}
	clear(a.pending)
	a.mu.Unlock()
	for _, event := range events {
		select {
		case a.out <- event:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func (a *Aggregator) emit(path string, pending *pendingEvent, token uint64) {
	a.mu.Lock()
	current, ok := a.pending[path]
	if !ok || a.closed || current != pending || current.token != token {
		a.mu.Unlock()
		return
	}
	delete(a.pending, path)
	event := ScanEvent{Path: path, Type: pending.eventType, DetectedAt: pending.lastSeen, OldPath: pending.oldPath}
	a.mu.Unlock()
	a.out <- event
}
