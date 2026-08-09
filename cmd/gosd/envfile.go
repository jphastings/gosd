package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/jphastings/gosd/internal/gosdtoml"
)

// parseEnvFile reads a gosd build --env-file and returns its verbatim [env]
// body (to splice into the card's gosd.toml) and the active (uncommented)
// entries to bake into config.json, plus any coercion warnings to surface.
//
// The file is the *body* of the [env] section — KEY = "value" lines and
// comments, exactly as they should appear on the card, including commented-out
// "suggested" entries — with no [env] header and no other TOML section (gosd
// frames the section itself). It's validated as TOML with no section headers so
// a bad file fails the build here rather than producing a gosd.toml the device
// can't parse; active keys follow the same rules as --env (envKeyPattern, no
// GOSD_*). An empty path returns zero values (no --env-file given).
func parseEnvFile(path string) (verbatim string, active map[string]string, warnings []string, err error) {
	if path == "" {
		return "", nil, nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, fmt.Errorf("reading --env-file %q failed: %w; check the path exists and is readable", path, err)
	}
	content := string(data)

	active, warnings, err = gosdtoml.ParseEnvBody(content)
	if err != nil {
		return "", nil, nil, fmt.Errorf("--env-file %q is invalid: %w", path, err)
	}

	for key := range active {
		switch {
		case !envKeyPattern.MatchString(key):
			return "", nil, nil, fmt.Errorf("--env-file %q key %q is invalid because it doesn't match [A-Za-z_][A-Za-z0-9_]*; use only letters, digits and underscores, and don't start with a digit", path, key)
		case strings.HasPrefix(key, "GOSD_"):
			return "", nil, nil, fmt.Errorf("--env-file %q key %s is invalid because GOSD_* names are reserved by gosd; rename it", path, key)
		}
	}

	return content, active, warnings, nil
}
