package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		ldflagsVersion string
		info           *debug.BuildInfo
		want           string
	}{
		{
			name:           "ldflags version wins",
			ldflagsVersion: "v1.2.3",
			info:           buildInfoVersion("v1.2.4", false),
			want:           "v1.2.3",
		},
		{
			name:           "ldflags version gets dirty suffix",
			ldflagsVersion: "v1.2.3",
			info:           buildInfoVersion("v1.2.4", true),
			want:           "v1.2.3+dirty",
		},
		{
			name:           "ldflags dirty suffix is not duplicated",
			ldflagsVersion: "v1.2.3+dirty",
			info:           buildInfoVersion("v1.2.4", true),
			want:           "v1.2.3+dirty",
		},
		{
			name:           "dev falls back to module version",
			ldflagsVersion: "dev",
			info:           buildInfoVersion("v0.0.0-20260612000000-abcdef123456", true),
			want:           "v0.0.0-20260612000000-abcdef123456",
		},
		{
			name:           "devel build info keeps dev",
			ldflagsVersion: "dev",
			info:           buildInfoVersion("(devel)", true),
			want:           "dev",
		},
		{
			name:           "empty version defaults to dev",
			ldflagsVersion: "",
			info:           nil,
			want:           "dev",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(tt.ldflagsVersion, tt.info); got != tt.want {
				t.Fatalf("resolveVersion(%q) = %q, want %q", tt.ldflagsVersion, got, tt.want)
			}
		})
	}
}

func buildInfoVersion(version string, modified bool) *debug.BuildInfo {
	info := &debug.BuildInfo{
		Main: debug.Module{
			Path:    "github.com/snjax/sya",
			Version: version,
		},
	}
	if modified {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: "vcs.modified", Value: "true"})
	}
	return info
}
