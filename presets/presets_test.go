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
