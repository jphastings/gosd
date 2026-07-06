package provision

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// parseUserData extracts the hostname field from cloud-init's user-data
// (a "#cloud-config" YAML document; the leading marker is a YAML comment
// and needs no special handling). Every other top-level field — packages,
// apt, timezone, keyboard, user, ssh_pwauth, runcmd, and anything else
// Imager's wizard or a hand-written file might add — is RPi-OS-specific
// and outside what gosd-init's minimal runtime consumes, so it's collected
// into ignored (sorted, for a deterministic single summary log line) rather
// than causing an error.
//
// An empty document (e.g. a zero-byte file) yields no hostname and no
// error. Anything that isn't a YAML mapping at the top level is reported as
// an error so the caller can log and fall back to the next source.
func parseUserData(data []byte) (hostname string, ignored []string, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", nil, fmt.Errorf("user-data is not valid YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return "", nil, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", nil, fmt.Errorf("user-data does not contain a YAML mapping at its top level")
	}

	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		if key == "hostname" {
			hostname = root.Content[i+1].Value
			continue
		}
		ignored = append(ignored, key)
	}
	sort.Strings(ignored)

	return hostname, ignored, nil
}
