package enhancer

import (
	"sync"
	"time"
)

type circuitState int

const (
	circuitClosed   circuitState = iota // healthy, requests pass through
	circuitOpen                         // tripped, requests short-circuit
	circuitHalfOpen                     // testing, one request allowed
)

// CircuitBreaker prevents cascading failures by short-circuiting after repeated errors.
// 3 consecutive failures → open for 60s → half-open probe.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        circuitState
	failures     int
	maxFailures  int
	openUntil    time.Time
	cooldown     time.Duration
}

// NewCircuitBreaker creates a circuit breaker with default settings.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: 3,
		cooldown:    60 * time.Second,
	}
}

// Allow checks whether a request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return true
	case circuitOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = circuitHalfOpen
			return true
		}
		return false
	case circuitHalfOpen:
		return false // only one probe at a time
	}
	return true
}

// RecordSuccess records a successful request. Resets the circuit breaker.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.state = circuitClosed
}

// RecordFailure records a failed request. May trip the circuit breaker.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	if cb.failures >= cb.maxFailures {
		cb.state = circuitOpen
		cb.openUntil = time.Now().Add(cb.cooldown)
	}
}

// State returns the current circuit breaker state as a string.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case circuitClosed:
		return "closed"
	case circuitOpen:
		if time.Now().After(cb.openUntil) {
			return "half-open"
		}
		return "open"
	case circuitHalfOpen:
		return "half-open"
	}
	return "unknown"
}
