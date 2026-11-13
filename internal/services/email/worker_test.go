package email

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type observedWorkerQueue struct {
	EmailQueue
	worker *Worker
	logs   *observer.ObservedLogs
	t      *testing.T
	err    error
}

func (q *observedWorkerQueue) ProcessBatch() error {
	entries := q.logs.All()
	if len(entries) != 1 || entries[0].Message != "Background job run started" {
		q.t.Error("expected run start log before processing the batch")
	}
	q.worker.Stop()
	return q.err
}

func TestWorkerRunDebugLogs(t *testing.T) {
	for _, batchErr := range []error{nil, errors.New("batch failed")} {
		name := "success"
		if batchErr != nil {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			restore := zap.ReplaceGlobals(zap.New(core))
			defer restore()
			queue := &observedWorkerQueue{logs: logs, t: t, err: batchErr}
			worker := NewWorker(queue, time.Millisecond)
			queue.worker = worker
			worker.Start()
			deadline := time.After(time.Second)
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for logs.Len() < 2 {
				select {
				case <-deadline:
					t.Fatal("timed out waiting for run logs")
				case <-ticker.C:
				}
			}
			entries := logs.All()
			for _, entry := range entries {
				if entry.Level != zap.DebugLevel || entry.ContextMap()["job"] != "email_queue" {
					t.Fatalf("unexpected run log: %+v", entry)
				}
			}
			finished := entries[1]
			if finished.Message != "Background job run finished" || finished.ContextMap()["elapsed"] == nil {
				t.Fatalf("missing completion timing: %+v", finished)
			}
			if batchErr != nil && finished.ContextMap()["error"] != batchErr.Error() {
				t.Fatalf("missing batch error: %+v", finished)
			}
		})
	}
}
