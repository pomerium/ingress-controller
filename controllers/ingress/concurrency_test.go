package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithMaxConcurrentReconciles(t *testing.T) {
	ic := &ingressController{}

	// default zero value leaves controller-runtime default in place
	assert.Equal(t, 0, ic.maxConcurrentReconciles)

	WithMaxConcurrentReconciles(4)(ic)
	assert.Equal(t, 4, ic.maxConcurrentReconciles)

	// a value < 1 is still recorded; SetupWithManager decides whether to apply it
	WithMaxConcurrentReconciles(0)(ic)
	assert.Equal(t, 0, ic.maxConcurrentReconciles)
}
