package launcher

import (
	"strings"

	"pissbox/host/config"
)

// secret is the shared configuration type used for CLI and global entries.
type secret = config.Secret

func (p *processes) checkSecrets() int {
	help, status := p.capture("sbx", []string{"secret", "set-custom", "--help"}, nil)
	if status != 0 {
		return fail("--secret requires SBX secret set-custom --command support (verified with v0.50.0).")
	}
	for _, flag := range []string{"--command", "--sandbox", "--env"} {
		if !strings.Contains(help, flag+" ") {
			return fail("--secret requires SBX secret set-custom %s support (verified with v0.50.0).", flag)
		}
	}
	return 0
}

func (p *processes) registerSecrets(name string, secrets []secret) int {
	for _, s := range secrets {
		if status := p.sbx("secret", "set-custom", "--sandbox", name, "--env", s.Env, "--host", s.Host, "--command", s.Command); status != 0 {
			return status
		}
	}
	return 0
}
