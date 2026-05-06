package certificate

import (
	"maps"
	"slices"
	"strings"
)

// NameIndex is used to index certificate DNS names. It supports exact and
// wildcard matching and dynamic updates.
type NameIndex[Key comparable] interface {
	// Add adds all the names to the index for the given key. Multiple
	// keys can be associated with the same names.
	Add(key Key, names []string)
	// Get returns all the names for the given key.
	Get(key Key) []string
	// Keys returns all the known keys. Order is non-deterministic.
	Keys() []Key
	// Lookup looks up any keys associated with the given name. Wildcard search
	// is supported (Lookup("*.example.com") -> would match a key for
	// "www.example.com"). Order is non-deterministic.
	Lookup(name string) []Key
	// Names returns all the known names. Order is non-deterministic.
	Names() []string
	// Remove removes a key from the index and possibly all of its associated
	// names if they aren't referenced by any other keys.
	Remove(key Key)
}

// NewNameIndex creates a new NameIndex for the given key type. The NameIndex
// is not safe for concurrent use.
func NewNameIndex[Key comparable]() NameIndex[Key] {
	return &nameIndex[Key]{
		suffixes: make(map[string]map[string]map[Key]struct{}),
		keys:     make(map[Key][]string),
	}
}

type nameIndex[Key comparable] struct {
	// suffixes is a map of suffix -> prefix -> keys,
	// where for www.example.com, suffix is example.com
	// and prefix is www
	suffixes map[string]map[string]map[Key]struct{}
	keys     map[Key][]string
}

func (idx *nameIndex[Key]) Add(key Key, names []string) {
	// always remove first to keep the index consistent
	idx.Remove(key)

	// add the key
	idx.keys[key] = slices.Clone(names)

	// index each of the names
	for _, name := range names {
		prefix, suffix := idx.splitName(name)

		prefixes, ok := idx.suffixes[suffix]
		if !ok {
			prefixes = make(map[string]map[Key]struct{})
			idx.suffixes[suffix] = prefixes
		}

		keys, ok := prefixes[prefix]
		if !ok {
			keys = make(map[Key]struct{})
			prefixes[prefix] = keys
		}

		keys[key] = struct{}{}
	}
}

func (idx *nameIndex[Key]) Get(key Key) []string {
	return slices.Clone(idx.keys[key])
}

func (idx *nameIndex[Key]) Keys() []Key {
	return slices.Collect(maps.Keys(idx.keys))
}

func (idx *nameIndex[Key]) Lookup(name string) []Key {
	prefix, suffix := idx.splitName(name)

	prefixes, ok := idx.suffixes[suffix]
	if !ok {
		return nil
	}

	allKeys := map[Key]struct{}{}
	// if the prefix is *, we should return all the keys for all the names
	if prefix == "*" {
		for _, keys := range prefixes {
			for key := range keys {
				allKeys[key] = struct{}{}
			}
		}
	} else {
		// lookup an exact match
		if keys, ok := prefixes[prefix]; ok {
			for key := range keys {
				allKeys[key] = struct{}{}
			}
		}
		// if we're storing a wildcard return those keys too
		if keys, ok := prefixes["*"]; ok {
			for key := range keys {
				allKeys[key] = struct{}{}
			}
		}
	}
	return slices.Collect(maps.Keys(allKeys))
}

func (idx *nameIndex[Key]) Names() []string {
	m := map[string]struct{}{}
	for _, names := range idx.keys {
		for _, name := range names {
			m[name] = struct{}{}
		}
	}
	return slices.Collect(maps.Keys(m))
}

func (idx *nameIndex[Key]) Remove(key Key) {
	names := idx.keys[key]
	delete(idx.keys, key)
	for _, name := range names {
		prefix, suffix := idx.splitName(name)

		prefixes, ok := idx.suffixes[suffix]
		if !ok {
			continue
		}

		keys, ok := prefixes[prefix]
		if !ok {
			continue
		}

		delete(keys, key)
		// remove empty maps
		if len(keys) == 0 {
			delete(prefixes, prefix)
		}
		if len(prefixes) == 0 {
			delete(idx.suffixes, suffix)
		}
	}
}

func (idx *nameIndex[Key]) splitName(name string) (prefix, suffix string) {
	// remove any trailing .
	name = strings.TrimSuffix(name, ".")
	// matching is lowercase
	name = strings.ToLower(name)

	prefix, suffix, found := strings.Cut(name, ".")
	if !found {
		return name, ""
	}

	return prefix, suffix
}
