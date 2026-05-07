package email

import (
	"time"
)

type Worker struct {
	Queue        EmailQueue
	StopCh       chan struct{}
	PollInterval time.Duration
}

func NewWorker(queue EmailQueue, pollInterval time.Duration) *Worker {
	return &Worker{Queue: queue, StopCh: make(chan struct{}), PollInterval: pollInterval}
}

func (w *Worker) Start() {
	go func() {
		ticker := time.NewTicker(w.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.Queue.ProcessBatch()
			case <-w.StopCh:
				return
			}
		}
	}()
}

func (w *Worker) Stop() {
	close(w.StopCh)
}
