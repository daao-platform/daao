package auth

// RateLimiterInterface abstracts in-memory and Redis-backed rate limiters.
// Satisfied by *RateLimiter (in-memory) and *RedisRateLimiter (enterprise).
type RateLimiterInterface interface {
	AllowSatellite(satelliteID string) bool
	AllowUser(userID string) bool
}
