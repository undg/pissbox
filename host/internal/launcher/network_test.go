package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pissbox/host/config"
)

func TestGlobalNetworkPermissionsReachCreationOnly(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "staged"}[staged], func(t *testing.T) {
			f := newFixture(t)
			f.write(f.userConfig, "[network]\nallow = ['*.example.com', 'api.example.com:8443']\nblock = ['blocked.example.com']\n")
			capture := filepath.Join(f.root, "network.json")
			env := map[string]string{"NETWORK_KIT_CAPTURE": capture}
			args := []string{"-y"}
			if staged {
				args = append(args, "--secret", "API", "api.example.com", "printf key")
			}
			out, status, calls := f.launch("", env, args...)
			if status != 0 {
				t.Fatalf("launch: %d %s", status, out)
			}
			var spec struct {
				SchemaVersion string `json:"schemaVersion"`
				Kind          string
				Name          string
				Permissions   struct {
					Network config.Network
				}
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			want := config.Network{Allow: []string{"*.example.com", "api.example.com:8443"}, Block: []string{"blocked.example.com"}}
			if spec.SchemaVersion != "2" || spec.Kind != "mixin" || spec.Name != "pissbox-network" || !reflect.DeepEqual(spec.Permissions.Network, want) {
				t.Fatalf("unexpected kit: %s", data)
			}
			seen := 0
			for _, call := range calls {
				kit := flagValue(call, "--kit")
				if kit == "" {
					continue
				}
				seen++
				verb := "run"
				if staged {
					verb = "create"
				}
				if call[0] != verb || flagValue(call, "--env") == "" {
					t.Fatalf("network kit passed outside creation: %v", call)
				}
				if _, err := os.Stat(kit); !os.IsNotExist(err) {
					t.Fatalf("temporary kit not cleaned: %s %v", kit, err)
				}
			}
			if seen != 1 {
				t.Fatalf("wanted one kit creation, got %d: %v", seen, calls)
			}
		})
	}
}

func TestGlobalNetworkConflictDoesNotChangeExistingSandbox(t *testing.T) {
	f := newFixture(t)
	f.write(f.userConfig, "[network]\nblock = ['example.com']\n")
	out, status, calls := f.launch("", map[string]string{"CONFLICT": "1"}, "-y")
	if status != 0 {
		t.Fatalf("attach: %d %s", status, out)
	}
	if flagValue(calls[0], "--kit") == "" {
		t.Fatal("creation missing network kit", calls)
	}
	for _, call := range calls[1:] {
		if flagValue(call, "--kit") != "" || call[0] == "policy" {
			t.Fatal("existing sandbox network modified", call)
		}
	}
}

func TestEmptyNetworkDoesNotCreateKit(t *testing.T) {
	kit, err := networkKit(filepath.Join(t.TempDir(), "missing"), config.Network{})
	if err != nil || kit != "" {
		t.Fatalf("empty network: %q %v", kit, err)
	}
}

func TestNetworkKitDirectoryFailure(t *testing.T) {
	_, err := networkKit(filepath.Join(t.TempDir(), "missing"), config.Network{Block: []string{"example.com"}})
	if err == nil {
		t.Fatal("missing state directory accepted")
	}
}
