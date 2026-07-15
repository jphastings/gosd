package extbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// cacheInputs is exactly the locked cache key recipe (bean gosd-sn30):
// script bytes, container image digest, arch, and output name. Marshaled
// to JSON (struct field order is deterministic) and hashed.
type cacheInputs struct {
	Script []byte
	Image  string
	Arch   string
	Name   string
}

// cacheKey computes the content-addressed cache key for building spec
// inside image.
func cacheKey(spec Spec, image string) (string, error) {
	in := cacheInputs{
		Script: spec.Script,
		Image:  image,
		Arch:   spec.Arch.Key(),
		Name:   spec.Name,
	}
	data, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("extbuild: hashing cache inputs: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// cacheComplete reports whether dir already holds this build's expected
// output and its source.json, i.e. whether Build can skip running the
// container entirely.
func cacheComplete(dir, name string) bool {
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, sourceJSONName)); err != nil {
		return false
	}
	return true
}
