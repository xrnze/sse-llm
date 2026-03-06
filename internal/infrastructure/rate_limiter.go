package infrastructure

import (
	"sync"
	"time"

	"sse-streaming-chat/internal/domain"
)

// tokenBucket represents a token bucket for rate limiting
type tokenBucket struct {
	tokens    int
	maxTokens int
	lastRefill time.Time
	refillRate time.Duration
}

// inMemoryRateLimiter implements domain.RateLimiter using in-memory token buckets
type inMemoryRateLimiter struct {
	buckets     map[string]*tokenBucket
	mutex       sync.RWMutex
	maxRequests int
	window      time.Duration
}

// NewInMemoryRateLimiter creates a new in-memory rate limiter
func NewInMemoryRateLimiter(maxRequests int, window time.Duration) domain.RateLimiter {
	limiter := &inMemoryRateLimiter{
		buckets:     make(map[string]*tokenBucket),
		maxRequests: maxRequests,
		window:      window,
	}
	
	// Start cleanup goroutine
	go limiter.cleanup()
	
	return limiter
}

// Allow checks if a request is allowed for the given key
func (r *inMemoryRateLimiter) Allow(key string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	bucket := r.getBucket(key)
	r.refillBucket(bucket)
	
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	
	return false
}

// GetLimit returns the current limit for a key
func (r *inMemoryRateLimiter) GetLimit(key string) int {
	return r.maxRequests
}

// GetRemaining returns remaining requests for a key
func (r *inMemoryRateLimiter) GetRemaining(key string) int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	bucket := r.getBucket(key)
	r.refillBucket(bucket)
	
	return bucket.tokens
}

// Reset resets the counter for a key
func (r *inMemoryRateLimiter) Reset(key string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	if bucket, exists := r.buckets[key]; exists {
		bucket.tokens = r.maxRequests
		bucket.lastRefill = time.Now()
	}
}

// getBucket gets or creates a token bucket for the given key
func (r *inMemoryRateLimiter) getBucket(key string) *tokenBucket {
	bucket, exists := r.buckets[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     r.maxRequests,
			maxTokens:  r.maxRequests,
			lastRefill: time.Now(),
			refillRate: r.window / time.Duration(r.maxRequests),
		}
		r.buckets[key] = bucket
	}
	
	return bucket
}

// refillBucket refills tokens based on elapsed time
func (r *inMemoryRateLimiter) refillBucket(bucket *tokenBucket) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	
	// Calculate how many tokens to add
	tokensToAdd := int(elapsed / bucket.refillRate)
	
	if tokensToAdd > 0 {
		bucket.tokens += tokensToAdd
		if bucket.tokens > bucket.maxTokens {
			bucket.tokens = bucket.maxTokens
		}
		bucket.lastRefill = now
	}
}

// cleanup removes old buckets to prevent memory leaks
func (r *inMemoryRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		r.mutex.Lock()
		
		now := time.Now()
		for key, bucket := range r.buckets {
			// Remove buckets that haven't been used for more than the window duration
			if now.Sub(bucket.lastRefill) > r.window*2 {
				delete(r.buckets, key)
			}
		}
		
		r.mutex.Unlock()
	}
}