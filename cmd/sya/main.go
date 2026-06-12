package main

import (
	"os"
	"runtime/debug"

	"github.com/snjax/sya/internal/cli"
)

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	app := cli.New(cli.Options{
		Version: resolveVersion(version, readBuildInfo()),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	os.Exit(app.Execute(os.Args[1:]))
}

func readBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

func resolveVersion(ldflagsVersion string, info *debug.BuildInfo) string {
	if ldflagsVersion != "" && ldflagsVersion != "dev" {
		if buildInfoSetting(info, "vcs.modified") == "true" && !hasDirtySuffix(ldflagsVersion) {
			return ldflagsVersion + "+dirty"
		}
		return ldflagsVersion
	}
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	if ldflagsVersion != "" {
		return ldflagsVersion
	}
	return "dev"
}

func buildInfoSetting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func hasDirtySuffix(version string) bool {
	return len(version) >= 6 && (version[len(version)-6:] == "+dirty" || version[len(version)-6:] == "-dirty")
}
