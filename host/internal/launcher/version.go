package launcher

import "runtime/debug"

// version is set at build time via -ldflags "-X pissbox/host/internal/launcher.version=...".
// Without an injected release, Go's VCS build stamp is used.
var version = "dev"

func resolveVersion() string {
	info, _ := debug.ReadBuildInfo()
	return versionFromBuildInfo(version, info)
}

// versionFromBuildInfo prefers the injected release version, then a short VCS
// revision with an optional dirty suffix. Missing VCS metadata means dev.
func versionFromBuildInfo(injected string, info *debug.BuildInfo) string {
	if injected != "dev" {
		return injected
	}
	if info == nil {
		return "dev"
	}

	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if dirty {
		revision += "-dirty"
	}
	return revision
}
