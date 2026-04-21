package install

import (
	"testing"

	"github.com/goodtiger/openclaw-install/internal/system"
)

func TestRecommendedMode(t *testing.T) {
	tests := []struct {
		name string
		info system.Info
		want Mode
	}{
		{
			name: "windows always docker",
			info: system.Info{OS: "windows"},
			want: ModeDocker,
		},
		{
			name: "docker healthy with compose",
			info: system.Info{OS: "linux", HasDocker: true, DockerHealthy: true, HasCompose: true},
			want: ModeDocker,
		},
		{
			name: "docker on PATH but daemon not running",
			info: system.Info{OS: "linux", HasDocker: true, DockerHealthy: false, HasCompose: true},
			want: ModeNative,
		},
		{
			name: "no docker at all",
			info: system.Info{OS: "linux", HasDocker: false, DockerHealthy: false, HasCompose: false},
			want: ModeNative,
		},
		{
			name: "docker healthy but no compose",
			info: system.Info{OS: "linux", HasDocker: true, DockerHealthy: true, HasCompose: false},
			want: ModeNative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RecommendedMode(tt.info)
			if got != tt.want {
				t.Errorf("RecommendedMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
