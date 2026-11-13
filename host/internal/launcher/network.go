package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"

	"pissbox/host/config"
)

// networkKit supplies additive permissions through a temporary v2 mixin. JSON
// is valid YAML; marshal patterns as data, never interpolate them into a spec.
func networkKit(state string, network config.Network) (string, error) {
	if len(network.Allow) == 0 && len(network.Block) == 0 {
		return "", nil
	}
	kit, err := os.MkdirTemp(state, "network-kit.")
	if err != nil {
		return "", err
	}
	spec := map[string]any{
		"schemaVersion": "2",
		"kind":          "mixin",
		"name":          "pissbox-network",
		"permissions":   map[string]any{"network": network},
	}
	data, err := json.Marshal(spec)
	if err == nil {
		err = os.WriteFile(filepath.Join(kit, "spec.yaml"), data, 0600)
	}
	if err != nil {
		os.RemoveAll(kit)
		return "", err
	}
	return kit, nil
}
