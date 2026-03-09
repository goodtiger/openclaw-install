package install

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/goodtiger/openclaw-install/presets"
)

type MirrorSelection map[string]presets.MirrorCandidate

func (w *Workflow) ResolveMirrors(ctx context.Context) (MirrorSelection, []string) {
	selection := make(MirrorSelection, len(w.Presets.Mirrors.Categories))
	warnings := []string{}

	keys := make([]string, 0, len(w.Presets.Mirrors.Categories))
	for key := range w.Presets.Mirrors.Categories {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		candidates := w.Presets.Mirrors.Categories[key]
		chosen, err := w.chooseMirror(ctx, key, candidates)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
		selection[key] = chosen
	}

	return selection, warnings
}

func (w *Workflow) chooseMirror(ctx context.Context, category string, candidates []presets.MirrorCandidate) (presets.MirrorCandidate, error) {
	if len(candidates) == 0 {
		return presets.MirrorCandidate{}, fmt.Errorf("mirror category %s has no candidates", category)
	}

	for _, candidate := range candidates {
		if candidate.ProbeURL == "" {
			return candidate, nil
		}
		if err := w.probeURL(ctx, candidate.ProbeURL); err == nil {
			return candidate, nil
		}
	}

	return candidates[0], fmt.Errorf("mirror category %s fell back to %s after probe failures", category, candidates[0].Name)
}

func (w *Workflow) probeURL(ctx context.Context, rawURL string) error {
	client := w.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("probe %s returned HTTP %d", rawURL, resp.StatusCode)
}
