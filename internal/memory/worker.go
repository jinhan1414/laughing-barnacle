package memory

import (
	"sync"
	"time"
)

type Worker struct {
	store       *Store
	interval    time.Duration
	idleWindow  time.Duration
	maxWindow   time.Duration
	maxMessages int

	mu      sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
	started bool
}

func NewWorker(store *Store, interval, idleWindow, maxWindow time.Duration, maxMessages int) *Worker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if idleWindow <= 0 {
		idleWindow = 5 * time.Minute
	}
	if maxWindow <= 0 {
		maxWindow = 10 * time.Minute
	}
	if maxMessages <= 0 {
		maxMessages = 8
	}
	return &Worker{
		store:       store,
		interval:    interval,
		idleWindow:  idleWindow,
		maxWindow:   maxWindow,
		maxMessages: maxMessages,
	}
}

func (w *Worker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.store == nil {
		return
	}
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.started = true
	go w.loop()
}

func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	close(w.stopCh)
	done := w.doneCh
	w.started = false
	w.mu.Unlock()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func (w *Worker) loop() {
	defer close(w.doneCh)
	w.runOnce()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.runOnce()
		case <-w.stopCh:
			return
		}
	}
}

func (w *Worker) runOnce() {
	now := time.Now().UTC()
	_, _ = w.store.CloseIdleSegments(now, w.idleWindow, w.maxWindow, w.maxMessages)
	_ = w.store.ProcessClosedSegments(now)
}
