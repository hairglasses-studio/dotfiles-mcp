package enhancer

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker()
	if cb.State() != "closed" {
		t.Errorf("expected initial state closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected Allow() to return true when closed")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker()

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Errorf("expected closed after 2 failures, got %s", cb.State())
	}

	cb.RecordFailure() // 3rd failure → open
	if cb.State() != "open" {
		t.Errorf("expected open after 3 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Error("expected Allow() to return false when open")
	}
}

func TestCircuitBreaker_SuccessResets(t *testing.T) {
	cb := NewCircuitBreaker()

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // reset

	if cb.State() != "closed" {
		t.Errorf("expected closed after success, got %s", cb.State())
	}

	// Should need 3 more failures to open again
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Errorf("expected still closed after 2 failures post-reset, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenAfterCooldown(t *testing.T) {
	cb := &CircuitBreaker{
		maxFailures: 3,
		cooldown:    10 * time.Millisecond, // very short for testing
	}

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Allow() {
		t.Error("expected Allow() false when open")
	}

	time.Sleep(15 * time.Millisecond) // wait for cooldown

	if !cb.Allow() {
		t.Error("expected Allow() true in half-open state")
	}
	if cb.State() != "half-open" {
		t.Errorf("expected half-open, got %s", cb.State())
	}

	// Second call in half-open should be denied
	if cb.Allow() {
		t.Error("expected Allow() false for second call in half-open")
	}
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	cb := &CircuitBreaker{
		maxFailures: 3,
		cooldown:    10 * time.Millisecond,
	}

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow() // transitions to half-open

	cb.RecordSuccess() // should close
	if cb.State() != "closed" {
		t.Errorf("expected closed after half-open success, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := &CircuitBreaker{
		maxFailures: 3,
		cooldown:    10 * time.Millisecond,
	}

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow() // transitions to half-open

	cb.RecordFailure() // failure count is now 4, >= 3
	if cb.State() != "open" {
		t.Errorf("expected open after half-open failure, got %s", cb.State())
	}
}
