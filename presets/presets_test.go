package presets

import "testing"

func TestLoad(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(bundle.Mirrors.Categories) == 0 {
		t.Fatal("expected mirror categories to be loaded")
	}

	if _, ok := bundle.ProviderByID("deepseek"); !ok {
		t.Fatal("expected deepseek provider preset to exist")
	}

	if _, ok := bundle.ChannelByID("qq"); !ok {
		t.Fatal("expected qq channel preset to exist")
	}
}

func TestMirrorPriorityPrefersChinaFriendlyCandidates(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cases := map[string]string{
		"docker_image":   "daocloud",
		"github_release": "ghproxy",
		"go_proxy":       "goproxy-cn",
		"npm_registry":   "npmmirror",
	}

	for category, want := range cases {
		candidates := bundle.Mirrors.Categories[category]
		if len(candidates) == 0 {
			t.Fatalf("expected candidates for %s", category)
		}
		if got := candidates[0].Name; got != want {
			t.Fatalf("%s first candidate = %q, want %q", category, got, want)
		}
	}
}
