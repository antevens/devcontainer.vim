package devcontainer

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMergeClaudeStopHook_EmptySettings(t *testing.T) {
	out, err := mergeClaudeStopHook(nil, "bash /home/vscode/.claude/hooks/log-response.sh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	hooks := got["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	if len(stop) != 1 {
		t.Fatalf("expected 1 Stop hook, got %d", len(stop))
	}
	entry := stop[0].(map[string]interface{})
	if entry["type"] != "command" {
		t.Errorf("hook type = %q, want \"command\"", entry["type"])
	}
	if !strings.Contains(entry["command"].(string), "log-response.sh") {
		t.Errorf("hook command %q does not reference log-response.sh", entry["command"])
	}
}

func TestMergeClaudeStopHook_PreservesExisting(t *testing.T) {
	existing := []byte(`{"hooks":{"Stop":[{"type":"command","command":"echo hello"}]}}`)
	out, err := mergeClaudeStopHook(existing, "bash /home/vscode/.claude/hooks/log-response.sh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	stop := got["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(stop) != 2 {
		t.Fatalf("expected 2 Stop hooks (existing + new), got %d", len(stop))
	}
}

func TestMergeClaudeStopHook_Idempotent(t *testing.T) {
	cmd := "bash /home/vscode/.claude/hooks/log-response.sh"
	first, err := mergeClaudeStopHook(nil, cmd)
	if err != nil {
		t.Fatalf("first merge error: %v", err)
	}
	second, err := mergeClaudeStopHook(first, cmd)
	if err != nil {
		t.Fatalf("second merge error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(second, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	stop := got["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(stop) != 1 {
		t.Errorf("expected 1 Stop hook after double merge (idempotent), got %d", len(stop))
	}
}

func TestMergeClaudeStopHook_InvalidJSONFallsBackToEmpty(t *testing.T) {
	out, err := mergeClaudeStopHook([]byte("not json"), "bash /home/vscode/.claude/hooks/log-response.sh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	stop := got["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(stop) != 1 {
		t.Errorf("expected 1 Stop hook after invalid-JSON input, got %d", len(stop))
	}
}

func TestTransferMicroK8sConfigNotInstalled(t *testing.T) {
	original := os.Getenv("PATH")
	os.Setenv("PATH", t.TempDir()) // empty dir — no microk8s binary
	defer os.Setenv("PATH", original)

	if err := transferMicroK8sConfig("fake-container", "/home/vscode"); err != nil {
		t.Errorf("expected nil when microk8s not in PATH, got: %v", err)
	}
}

func TestIsVimrcTarget(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"{{ remoteEnv:HOME }}/.vimrc", true},
		{"{{ remoteEnv:HOME }}/vimrc", true},
		{"/home/vscode/.vimrc", true},
		{"/home/vscode/vimrc", true},
		{"{{ remoteEnv:HOME }}/.vim/init.vim", true},
		{"/home/vscode/.config/nvim/init.vim", true},
		{"{{ remoteEnv:HOME }}/.bashrc", false},
		{"{{ remoteEnv:HOME }}/.vim", false},
		{"/home/vscode/.gitconfig", false},
		{"/home/vscode/.config/nvim/init.lua", false},
	}
	for _, tc := range tests {
		got := isVimrcTarget(tc.target)
		if got != tc.want {
			t.Errorf("isVimrcTarget(%q) = %v, want %v", tc.target, got, tc.want)
		}
	}
}

func TestMergeVimrcContent(t *testing.T) {
	dir := t.TempDir()

	sourceFile := dir + "/source.vim"
	additionalFile := dir + "/additional.vim"

	sourceContent := "\" user settings\nset number\nset expandtab\n"
	additionalContent := "\" devcontainer.vim keybindings\nnnoremap <silent> \"*yy yy\n"

	if err := os.WriteFile(sourceFile, []byte(sourceContent), 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}
	if err := os.WriteFile(additionalFile, []byte(additionalContent), 0644); err != nil {
		t.Fatalf("failed to write additional file: %v", err)
	}

	combined, err := mergeVimrcContent(sourceFile, additionalFile)
	if err != nil {
		t.Fatalf("mergeVimrcContent returned error: %v", err)
	}

	result := string(combined)
	if !strings.Contains(result, sourceContent) {
		t.Errorf("merged content is missing source vimrc: %q", result)
	}
	if !strings.Contains(result, additionalContent) {
		t.Errorf("merged content is missing additional vimrc template: %q", result)
	}

	sourceIdx := strings.Index(result, sourceContent)
	additionalIdx := strings.Index(result, additionalContent)
	if sourceIdx > additionalIdx {
		t.Errorf("source content should appear before additional content")
	}
}

func TestMergeVimrcContentMissingSoure(t *testing.T) {
	dir := t.TempDir()
	additionalFile := dir + "/additional.vim"
	if err := os.WriteFile(additionalFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write additional file: %v", err)
	}

	_, err := mergeVimrcContent(dir+"/nonexistent.vim", additionalFile)
	if err == nil {
		t.Error("expected error for missing source file, got nil")
	}
}

func TestMergeVimrcContentMissingAdditional(t *testing.T) {
	dir := t.TempDir()
	sourceFile := dir + "/source.vim"
	if err := os.WriteFile(sourceFile, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	_, err := mergeVimrcContent(sourceFile, dir+"/nonexistent.vim")
	if err == nil {
		t.Error("expected error for missing additional file, got nil")
	}
}
