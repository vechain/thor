package co

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestChoesGoAndWait(t *testing.T) {
	g := NewChoes()
	var counter int32

	// Start a simple goroutine that increments a counter
	g.Go(func(stopChan chan struct{}) {
		for i := 0; i < 10; i++ {
			select {
			case <-stopChan:
				return
			default:
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	g.Wait()

	if counter != 10 {
		t.Errorf("Expected counter to be 10, got %d", counter)
	}
}

func TestChoesStop(t *testing.T) {
	g := NewChoes()
	var counter int32

	// Start a goroutine that increments a counter indefinitely until stopped
	g.Go(func(stopChan chan struct{}) {
		for {
			select {
			case <-stopChan:
				return
			default:
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	// Let the goroutine run for a short time
	time.Sleep(50 * time.Millisecond)

	// Signal the goroutine to stop
	g.Stop()

	// Wait for all goroutines to finish
	g.Wait()

	finalCount := atomic.LoadInt32(&counter)
	if finalCount <= 0 {
		t.Errorf("Expected counter to be greater than 0, got %d", finalCount)
	}

	// Verify that the goroutine has stopped incrementing the counter
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&counter) != finalCount {
		t.Errorf("Counter changed after Stop was called, expected %d, got %d", finalCount, atomic.LoadInt32(&counter))
	}
}

func TestChoesStopMultipleCalls(t *testing.T) {
	g := NewChoes()
	var counter int32

	// Start a goroutine that increments a counter
	g.Go(func(stopChan chan struct{}) {
		for {
			select {
			case <-stopChan:
				return
			default:
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	// Stop the goroutine
	g.Stop()
	// Try stopping again (should have no effect)
	g.Stop()

	// Wait for all goroutines to finish
	g.Wait()

	// Ensure the counter has stopped incrementing
	finalCount := atomic.LoadInt32(&counter)
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&counter) != finalCount {
		t.Errorf("Counter changed after Stop was called twice, expected %d, got %d", finalCount, atomic.LoadInt32(&counter))
	}
}

func TestChoesStopFromOutside(t *testing.T) {
	g := NewChoes()
	var counter int32

	// Start a goroutine that increments a counter
	g.Go(func(stopChan chan struct{}) {
		for {
			select {
			case <-stopChan:
				return
			default:
				atomic.AddInt32(&counter, 1)
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	// Start another goroutine that stops the first one
	go func() {
		time.Sleep(50 * time.Millisecond)
		g.Stop()
	}()

	// Wait for all goroutines to finish
	g.Wait()

	finalCount := atomic.LoadInt32(&counter)
	if finalCount <= 0 {
		t.Errorf("Expected counter to be greater than 0, got %d", finalCount)
	}

	// Verify that the goroutine has stopped incrementing the counter
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&counter) != finalCount {
		t.Errorf("Counter changed after Stop was called from outside, expected %d, got %d", finalCount, atomic.LoadInt32(&counter))
	}
}
