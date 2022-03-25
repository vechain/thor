// Copyright (c) 2022 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

// worker is the simple worker to asynchronously run tasks.
// It has only one background goroutine and all tasks are executed one by one.
type worker struct {
	taskCh chan func() error
	ackCh  chan error
}

// NewWorker creates a worker.
func newWorker() *worker {
	w := &worker{
		taskCh: make(chan func() error, 16),
		ackCh:  make(chan error),
	}
	go w.worker()
	return w
}

// Close closes the worker and stops the background goroutine.
func (w *worker) Close() {
	close(w.taskCh)
	<-w.ackCh
}

// Run pushes the task into the background queue.
func (w *worker) Run(task func() error) {
	if task != nil {
		w.taskCh <- task
	}
}

// Sync ensures the last task has been executed.
func (w *worker) Sync() error {
	w.taskCh <- nil
	return <-w.ackCh
}

func (w *worker) worker() {
	defer func() {
		close(w.ackCh)
	}()
	var err error
	for task := range w.taskCh {
		if task != nil {
			if err == nil {
				err = task()
			}
		} else {
			w.ackCh <- err
		}
	}
}
