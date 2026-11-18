package config

import _ "embed"

//go:embed seed.json
var seedJSON []byte

// Seed returns the built-in categories. The binary always works
// even when no categories.json file sits next to it: the seed is
// embedded and used as a fallback.
func Seed() (*File, error) {
	return Parse(seedJSON)
}
