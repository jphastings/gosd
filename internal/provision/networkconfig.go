package provision

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// parseNetworkConfig extracts every WiFi access point named under
// network-config's netplan-style network.wifis.<interface>.access-points
// map, across every wifi interface listed, in file order (map order isn't
// preserved by yaml.v3 when unmarshaling into a Go map, so this walks the
// raw yaml.Node tree instead — the only way to honor the "multiple APs =
// take all, ordered" requirement).
//
// Sections gosd-init doesn't consume (ethernets, regulatory-domain, dhcp4,
// optional, ...) are simply not visited. A network-config with no
// network.wifis section at all (e.g. Ethernet-only, or hostname-only
// scenarios where Imager never wrote this file) yields a nil slice and no
// error — that's the normal, common case.
func parseNetworkConfig(data []byte) ([]WifiNetwork, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("network-config is not valid YAML: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}

	root := doc.Content[0]
	networkNode, ok := mapLookup(root, "network")
	if !ok {
		return nil, nil
	}
	wifisNode, ok := mapLookup(networkNode, "wifis")
	if !ok {
		return nil, nil
	}
	if wifisNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("network.wifis is not a YAML mapping")
	}

	var networks []WifiNetwork
	for i := 0; i+1 < len(wifisNode.Content); i += 2 {
		ifaceNode := wifisNode.Content[i+1]
		apsNode, ok := mapLookup(ifaceNode, "access-points")
		if !ok {
			continue
		}
		if apsNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("access-points is not a YAML mapping")
		}

		for j := 0; j+1 < len(apsNode.Content); j += 2 {
			ssidNode := apsNode.Content[j]
			apNode := apsNode.Content[j+1]

			var ap struct {
				Password string `yaml:"password"`
				Hidden   bool   `yaml:"hidden"`
			}
			if err := apNode.Decode(&ap); err != nil {
				return nil, fmt.Errorf("decoding access point %q: %w", ssidNode.Value, err)
			}

			networks = append(networks, WifiNetwork{
				SSID:     ssidNode.Value,
				Password: ap.Password,
				Hidden:   ap.Hidden,
			})
		}
	}

	return networks, nil
}

// mapLookup returns the value node for key within node, a YAML mapping
// node, along with whether it was found. node not being a mapping at all
// (or nil) is treated as "not found" rather than an error, so callers can
// simply stop descending when an optional section is absent.
func mapLookup(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}
