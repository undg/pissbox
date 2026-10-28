package launcher

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode"
)

type secret struct {
	env, host, command string
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s secret) validate(existing []secret) error {
	if !envName.MatchString(s.env) || strings.HasPrefix(s.env, "PISSBOX_") || s.env == "SANDBOX_NAME" {
		return fmt.Errorf("--secret ENV_NAME must be a non-reserved environment variable name")
	}
	if s.host == "" || strings.HasPrefix(s.host, "-") || strings.ContainsAny(s.host, "/\\?#@") ||
		strings.ContainsFunc(s.host, unicode.IsSpace) || (strings.Contains(s.host, ":") && net.ParseIP(s.host) == nil) {
		return fmt.Errorf("--secret HOST must be a host, IP, or wildcard pattern, not a URL or host:port")
	}
	if strings.TrimSpace(s.command) == "" || strings.HasPrefix(s.command, "-") {
		return fmt.Errorf("--secret COMMAND must be a non-empty quoted command")
	}
	for _, previous := range existing {
		if previous.env == s.env {
			return fmt.Errorf("--secret ENV_NAME must not be repeated: %s", s.env)
		}
	}
	return nil
}

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
		if status := p.sbx("secret", "set-custom", "--sandbox", name, "--env", s.env, "--host", s.host, "--command", s.command); status != 0 {
			return status
		}
	}
	return 0
}
