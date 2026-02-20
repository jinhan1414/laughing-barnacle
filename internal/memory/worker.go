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
	trashTTL    time.Duration
	retryAfter  time.Duration

	mu      sync.Mutex
	stopCh  chan struct{}
	doneCh  chan struct{}
	started bool
}

func NewWorker(store *Store, interval, idleWindow, maxWindow time.Duration, maxMessages int, trashTTL, retryAfter time.Duration) *Worker {
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
	if trashTTL <= 0 {
		trashTTL = 30 * 24 * time.Hour
	}
	if retryAfter <= 0 {
		retryAfter = 2 * time.Minute
	}
	return &Worker{
		store:       store,
		interval:    interval,
		idleWindow:  idleWindow,
		maxWindow:   maxWindow,
		maxMessages: maxMessages,
		trashTTL:    trashTTL,
		retryAfter:  retryAfter,
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
	_, _ = w.store.RunMaintenance(now, w.trashTTL, w.retryAfter)
}
