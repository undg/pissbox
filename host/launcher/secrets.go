package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"pissbox/host/config"
)

// secret is the shared configuration type used for CLI and global entries.
type secret = config.Secret

// registeredSecret pairs a configured secret with the placeholder known to the
// sandbox. The launcher injects the environment during staged creation, then
// registers the same placeholder without an SBX-managed environment binding.
type registeredSecret struct {
	secret
	Placeholder string
}

func (p *processes) checkSbxSecretFlag() int {
	help, status := p.capture("sbx", []string{"secret", "set-custom", "--help"}, nil)
	if status != 0 {
		return fail("--secret requires SBX secret set-custom --command/--placeholder support (verified with v0.50.0).")
	}
	for _, flag := range []string{"--command", "--sandbox", "--placeholder"} {
		if !strings.Contains(help, flag+" ") {
			return fail("--secret requires SBX secret set-custom %s support (verified with v0.50.0).", flag)
		}
	}
	return 0
}

func prepareSecrets(secrets []secret) ([]registeredSecret, error) {
	registrations := make([]registeredSecret, 0, len(secrets))
	for _, s := range secrets {
		for {
			id := make([]byte, 16)
			if _, err := rand.Read(id); err != nil {
				return nil, fmt.Errorf("generate secret placeholder: %w", err)
			}
			placeholder := "sbx-cs-" + hex.EncodeToString(id)
			duplicate := false
			for _, previous := range registrations {
				if previous.Placeholder == placeholder {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			registrations = append(registrations, registeredSecret{secret: s, Placeholder: placeholder})
			break
		}
	}
	return registrations, nil
}

func (p *processes) registerSecrets(name string, secrets []registeredSecret) int {
	for _, s := range secrets {
		// Creation already injected Env=Placeholder. Binding --env here would
		// conflict with registrations surviving an older sandbox of this name.
		args := []string{"secret", "set-custom", "--sandbox", name}
		for _, host := range s.Hosts {
			args = append(args, "--host", host)
		}
		args = append(args, "--placeholder", s.Placeholder, "--command", s.Command)
		if status := p.sbx(args...); status != 0 {
			return status
		}
	}
	return 0
}
