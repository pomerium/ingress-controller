package envoy

import (
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidationCache(t *testing.T) {
	c := &validationCache{}
	h1 := sha256.Sum256([]byte("config-1"))
	h2 := sha256.Sum256([]byte("config-2"))

	// empty cache: never a hit
	assert.False(t, c.hit(h1), "empty cache should not hit")

	// after storing h1, only h1 hits
	c.store(h1)
	assert.True(t, c.hit(h1), "stored hash should hit")
	assert.False(t, c.hit(h2), "different hash should miss")

	// storing h2 replaces h1 (single-slot cache)
	c.store(h2)
	assert.True(t, c.hit(h2), "newly stored hash should hit")
	assert.False(t, c.hit(h1), "previous hash should no longer hit")
}

func TestValidationCacheConcurrent(t *testing.T) {
	c := &validationCache{}
	h := sha256.Sum256([]byte("config"))
	c.store(h)

	// concurrent readers and writers must not race (run with -race).
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = c.hit(h) }()
		go func() { defer wg.Done(); c.store(h) }()
	}
	wg.Wait()
	assert.True(t, c.hit(h))
}
