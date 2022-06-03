package util

import "fmt"

// MergeMaps is used to merge configmap and secret values
func MergeMaps(first map[string]string, second map[string][]byte) (map[string]string, error) {
	dst := make(map[string]string)
	for key, val := range first {
		dst[key] = val
	}
	for key, data := range second {
		if _, there := first[key]; there {
			return nil, fmt.Errorf("secret contains key %s that was already specified by a non-secret rule", key)
		}
		dst[key] = string(data)
	}
	return dst, nil
}
