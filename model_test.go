package main

import (
	"strings"
	"testing"
)

const samplePatch = `# header comment
- id: ui-settings-general
  name: "@deepseek-ai/dsh-client-ui-settings-general"
  config:
    welcomeNoticeVersion: 2026-09-28.1
- id: agent-default-model
  name: "@deepseek-ai/dsh-agent-default-model"
  config:
    provider: deepseek-account
    model: deepseek-flash
    reasoningEffort: high
`

func TestRewriteExistingBlock(t *testing.T) {
	out, changed := rewriteDefaultModel(samplePatch, "glm-5.3-flash-free")
	if !changed {
		t.Fatal("expected change")
	}
	for _, want := range []string{
		"# header comment",
		"welcomeNoticeVersion: 2026-09-28.1",
		"provider: opencode-bridge",
		"model: glm-5.3-flash-free",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "reasoningEffort") {
		t.Error("reasoningEffort should be removed")
	}
	if strings.Contains(out, "provider: deepseek-account") {
		t.Error("old provider should be gone")
	}
}

func TestRewriteAppendsWhenMissing(t *testing.T) {
	out, changed := rewriteDefaultModel("- id: other\n  name: x\n", "beam-free")
	if !changed {
		t.Fatal("expected change")
	}
	if !strings.Contains(out, "- id: agent-default-model") {
		t.Fatalf("block not appended:\n%s", out)
	}
	if !strings.Contains(out, "model: beam-free") {
		t.Fatalf("model not written:\n%s", out)
	}
	if !strings.Contains(out, "- id: other") {
		t.Fatal("existing entry lost")
	}
}

func TestRewriteReplacesEmptyArrayScaffold(t *testing.T) {
	scaffold := "# header comment\n# second line\n[]\n"
	out, changed := rewriteDefaultModel(scaffold, "deepseek-v4.1-flash-free")
	if !changed {
		t.Fatal("expected change")
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "[]" {
			t.Fatalf("empty array left behind:\n%s", out)
		}
	}
	if !strings.Contains(out, "- id: agent-default-model") || !strings.Contains(out, "model: deepseek-v4.1-flash-free") {
		t.Fatalf("block not appended:\n%s", out)
	}
}

func TestPickFallbackOrder(t *testing.T) {
	available := map[string]bool{
		"big-pickle":           true,
		"mimo-v2.6-flash-free": true,
	}
	if got := pickFallback(available); got != "mimo-v2.6-flash-free" {
		t.Fatalf("got %q, want mimo before big-pickle", got)
	}
	if got := pickFallback(map[string]bool{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestProfileFromArgs(t *testing.T) {
	cases := map[string][]string{
		"web":     {},
		"desktop": {"--profile", "desktop"},
		"tui":     {"--profile=tui"},
	}
	for want, args := range cases {
		if got := profileFromArgs(args); got != want {
			t.Errorf("profileFromArgs(%v) = %q, want %q", args, got, want)
		}
	}
}
