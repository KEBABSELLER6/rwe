package buildinfo

import "runtime/debug"

func Version(stamped string) string {
	if stamped != "dev" {
		return stamped
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return stamped
	}
	return info.Main.Version
}
