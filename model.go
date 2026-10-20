package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// free models in preference order, as requested
var fallbackModels = []string{
	"deepseek-v4.1-flash-free",
	"minimax-m3.1-flash-free",
	"mimo-v2.6-flash-free",
	"glm-5.3-flash-free",
	"nemotron-3.5-lightning-free",
	"longcat-2.5-preview-free",
	"beam-free",
	"big-pickle",
	"nemotron-3.5-super-vl-free",
}

const modelsURL = "https://opencode.ai/inference/v1/models"

func profileFromArgs(args []string) string {
	for i, a := range args {
		if a == "--profile" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--profile=") {
			return strings.TrimPrefix(a, "--profile=")
		}
	}
	return "web"
}

func patchPath(profile string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dsh", "profiles", profile, "cordis.patch.yml")
}

func fetchModelIDs() (map[string]bool, error) {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(modelsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(payload.Data))
	for _, m := range payload.Data {
		ids[m.ID] = true
	}
	return ids, nil
}

func pickFallback(available map[string]bool) string {
	for _, id := range fallbackModels {
		if available[id] {
			return id
		}
	}
	return ""
}

// rewriteDefaultModel points the agent-default-model entry at modelID while
// leaving every other line, comment and !!js expression untouched.
func rewriteDefaultModel(content, modelID string) (string, bool) {
	lines := strings.Split(content, "\n")

	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "- id: agent-default-model" {
			start = i
			break
		}
	}

	if start == -1 {
		block := "- id: agent-default-model\n" +
			"  name: \"@deepseek-ai/dsh-agent-default-model\"\n" +
			"  config:\n" +
			"    provider: opencode-bridge\n" +
			"    model: " + modelID + "\n"
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + block, true
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "- ") {
			end = i
			break
		}
	}

	configIdx, providerIdx, modelIdx := -1, -1, -1
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(trimmed, "config:"):
			configIdx = i
		case strings.HasPrefix(trimmed, "provider:"):
			providerIdx = i
		case strings.HasPrefix(trimmed, "model:"):
			modelIdx = i
		}
	}

	indent := "    "
	if configIdx >= 0 {
		base := lines[configIdx]
		pad := len(base) - len(strings.TrimLeft(base, " "))
		indent = strings.Repeat(" ", pad+2)
	}

	switch {
	case configIdx == -1:
		insert := []string{"  config:", indent + "provider: opencode-bridge", indent + "model: " + modelID}
		lines = append(lines[:end], append(insert, lines[end:]...)...)
	case providerIdx == -1 || modelIdx == -1:
		insert := []string{indent + "provider: opencode-bridge", indent + "model: " + modelID}
		lines = append(lines[:configIdx+1], append(insert, lines[configIdx+1:]...)...)
	default:
		lines[providerIdx] = indent + "provider: opencode-bridge"
		lines[modelIdx] = indent + "model: " + modelID
	}

	// a reasoning effort meant for another model can break the free ones
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i >= start && i < end && strings.HasPrefix(strings.TrimSpace(line), "reasoningEffort:") {
			continue
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n"), true
}

// ensureDefaultModel runs once per machine: if the profile still points at
// another provider, it picks the first fallback model the catalog actually
// serves and writes it to the patch. It never touches an already configured
// opencode-bridge profile, so a model that disappears later stays selected.
func ensureDefaultModel(profile string) string {
	path := patchPath(profile)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	content := string(data)
	if strings.Contains(content, "provider: opencode-bridge") {
		return ""
	}

	available, err := fetchModelIDs()
	if err != nil {
		return ""
	}
	model := pickFallback(available)
	if model == "" {
		return ""
	}

	updated, changed := rewriteDefaultModel(content, model)
	if !changed {
		return ""
	}

	backup := path + ".bak-model"
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		_ = os.WriteFile(backup, data, 0o644)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o644); err != nil {
		return ""
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return ""
	}
	return model
}
