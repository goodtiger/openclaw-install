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

	if _, ok := bundle.ChannelByID("wechat"); !ok {
		t.Fatal("expected wechat channel preset to exist")
	}

	if _, ok := bundle.ChannelByID("dingtalk"); !ok {
		t.Fatal("expected dingtalk channel preset to exist")
	}

	dt, ok := bundle.ChannelByID("dingtalk")
	if !ok {
		t.Fatal("dingtalk preset not found")
	}
	if dt.ConfigMethod != "config_set" {
		t.Fatalf("dingtalk config_method = %q, want %q", dt.ConfigMethod, "config_set")
	}
	if len(dt.RequiredFields) != 2 {
		t.Fatalf("dingtalk required_fields count = %d, want 2", len(dt.RequiredFields))
	}
	if dt.RequiredFields[0].Key != "clientId" {
		t.Fatalf("dingtalk required_fields[0].key = %q, want %q", dt.RequiredFields[0].Key, "clientId")
	}
	if dt.RequiredFields[1].Key != "clientSecret" {
		t.Fatalf("dingtalk required_fields[1].key = %q, want %q", dt.RequiredFields[1].Key, "clientSecret")
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
