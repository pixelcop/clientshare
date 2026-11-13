package email

import (
	"time"

	"go.uber.org/zap"
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
				logger := zap.L().With(zap.String("job", "email_queue"))
				started := time.Now()
				logger.Debug("Background job run started")
				err := w.Queue.ProcessBatch()
				logger.Debug("Background job run finished", zap.Duration("elapsed", time.Since(started)), zap.Error(err))
			case <-w.StopCh:
				return
			}
		}
	}()
}

func (w *Worker) Stop() {
	close(w.StopCh)
}
