package util_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/ingress-controller/util"
)

func TestMergeMap(t *testing.T) {
	for _, tc := range []struct {
		name        string
		src         map[string][]byte
		dst         map[string]string
		expect      map[string]string
		expectError bool
	}{
		{name: "nothing", src: nil, dst: nil, expect: map[string]string{}, expectError: false},
		{name: "key overlap", src: map[string][]byte{
			"k1": []byte("v1"),
		}, dst: map[string]string{
			"k1": "v1.1",
			"k2": "v2",
		}, expect: nil, expectError: true},
		{name: "no overlap", src: map[string][]byte{
			"k1": []byte("v1"),
		}, dst: map[string]string{
			"k2": "v2",
		}, expect: map[string]string{
			"k1": "v1",
			"k2": "v2",
		}, expectError: false},
	} {
		got, err := util.MergeMaps(tc.dst, tc.src)
		if tc.expectError {
			assert.Error(t, err, tc.name)
			continue
		}
		if assert.NoError(t, err, tc.name) {
			assert.Equal(t, tc.expect, got)
		}
	}
}
