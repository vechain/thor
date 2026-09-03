// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package node

import (
	"errors"
	"testing"
	"time"
)

// TestWorkerTaskExecution checks if tasks are being executed.
func TestWorkerTaskExecution(t *testing.T) {
	w := newWorker()
	defer w.Close()

	executed := false
	task := func() error {
		executed = true
		return nil
	}

	w.Run(task)
	w.Sync()

	if !executed {
		t.Errorf("Task was not executed")
	}
}

// TestWorkerOrderOfExecution checks if tasks are executed in the correct order.
func TestWorkerOrderOfExecution(t *testing.T) {
	w := newWorker()
	defer w.Close()

	var order []int
	task1 := func() error {
		order = append(order, 1)
		return nil
	}
	task2 := func() error {
		order = append(order, 2)
		return nil
	}

	w.Run(task1)
	w.Run(task2)
	w.Sync()

	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("Tasks were executed in the wrong order")
	}
}

// TestWorkerErrorHandling checks how the worker handles task errors.
func TestWorkerErrorHandling(t *testing.T) {
	w := newWorker()
	defer w.Close()

	expectedError := errors.New("error")
	task := func() error {
		return expectedError
	}

	w.Run(task)
	err := w.Sync()

	if err != expectedError {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
}

// TestWorkerClosure checks if the worker stops correctly on closure.
func TestWorkerClosure(t *testing.T) {
	w := newWorker()

	closed := make(chan struct{})
	go func() {
		w.Close()
		close(closed)
	}()

	select {
	case <-closed:
		// Success
	case <-time.After(time.Second):
		t.Errorf("Worker did not close in time")
	}
}
