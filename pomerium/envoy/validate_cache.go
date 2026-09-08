// Package envoy contains functions for working with an embedded envoy binary.
package envoy

import (
	"crypto/sha256"
	"sync"
)

// defaultValidationCache is the process-wide cache of the last successfully
// validated config hash. It is shared across concurrent reconciles.
var defaultValidationCache = &validationCache{}

// validationCache caches the hash of the most recently successfully-validated
// config so that repeated validation of an unchanged config can skip the
// expensive embedded-envoy subprocess. It is safe for concurrent use.
type validationCache struct {
	mu       sync.RWMutex
	lastHash [sha256.Size]byte
	valid    bool
}

// hit reports whether hash matches the last successfully-validated config.
func (c *validationCache) hit(hash [sha256.Size]byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.valid && c.lastHash == hash
}

// store records hash as the last successfully-validated config.
func (c *validationCache) store(hash [sha256.Size]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastHash, c.valid = hash, true
}
