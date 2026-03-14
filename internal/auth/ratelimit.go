package auth

import (
	"sync"
	"time"
)

// RateLimiter implements per-satellite and per-user rate limiting
type RateLimiter struct {
	mu              sync.RWMutex
	satelliteLimits map[string]*tokenBucket
	userLimits      map[string]*tokenBucket
}

type tokenBucket struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		satelliteLimits: make(map[string]*tokenBucket),
		userLimits:      make(map[string]*tokenBucket),
	}
}

// AllowSatellite checks if a satellite is allowed to make a request
func (r *RateLimiter) AllowSatellite(satelliteID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, exists := r.satelliteLimits[satelliteID]
	if !exists {
		bucket = &tokenBucket{maxTokens: 100, tokens: 100, refillRate: time.Second, lastRefill: time.Now()}
		r.satelliteLimits[satelliteID] = bucket
	}

	now := time.Now()
	if now.Sub(bucket.lastRefill) >= bucket.refillRate {
		bucket.tokens = bucket.maxTokens
		bucket.lastRefill = now
	}

	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

// AllowUser checks if a user is allowed to make a request
func (r *RateLimiter) AllowUser(userID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, exists := r.userLimits[userID]
	if !exists {
		bucket = &tokenBucket{maxTokens: 50, tokens: 50, refillRate: time.Second, lastRefill: time.Now()}
		r.userLimits[userID] = bucket
	}

	now := time.Now()
	if now.Sub(bucket.lastRefill) >= bucket.refillRate {
		bucket.tokens = bucket.maxTokens
		bucket.lastRefill = now
	}

	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

var _ RateLimiterInterface = (*RateLimiter)(nil)
