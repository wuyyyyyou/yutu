// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestNormalizeExecutaCommand(t *testing.T) {
	t.Parallel()

	t.Run("strips binary prefix", func(t *testing.T) {
		t.Parallel()

		got, err := normalizeExecutaCommand("/tmp/yutu", []string{"yutu", "search", "list"})
		if err != nil {
			t.Fatalf("normalizeExecutaCommand() error = %v", err)
		}

		want := []string{"search", "list"}
		if len(got) < len(want) {
			t.Fatalf("normalizeExecutaCommand() got %v, want prefix %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("normalizeExecutaCommand() got %v, want prefix %v", got, want)
			}
		}
	})

	t.Run("blocks recursive commands", func(t *testing.T) {
		t.Parallel()

		_, err := normalizeExecutaCommand("/tmp/yutu", []string{"executa"})
		if err == nil {
			t.Fatal("normalizeExecutaCommand() expected error for blocked command")
		}
	})
}

func TestEnsureJSONOutput(t *testing.T) {
	tempParent := &cobra.Command{Use: "executa-test-parent"}
	tempChild := &cobra.Command{Use: "run"}
	tempChild.Flags().String("output", "", "output format")
	tempParent.AddCommand(tempChild)
	RootCmd.AddCommand(tempParent)
	defer RootCmd.RemoveCommand(tempParent)

	got := ensureJSONOutput([]string{"executa-test-parent", "run"})
	want := []string{"executa-test-parent", "run", "--output", "json"}
	if len(got) != len(want) {
		t.Fatalf("ensureJSONOutput() got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ensureJSONOutput() got %v, want %v", got, want)
		}
	}
}

func TestBuildExecutaEnvWithAuthorizedUserFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	authFile := filepath.Join(dir, "authorized_user.json")
	content := `{"client_id":"cid","client_secret":"sec","refresh_token":"ref","type":"authorized_user"}`
	if err := os.WriteFile(authFile, []byte(content), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	envs, err := buildExecutaEnv(map[string]any{executaAuthorizedUserCredential: authFile})
	if err != nil {
		t.Fatalf("buildExecutaEnv() error = %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("buildExecutaEnv() got %d envs, want 2", len(envs))
	}

	var gotCredential map[string]any
	if err := json.Unmarshal([]byte(envs[0][len("YUTU_CREDENTIAL="):]), &gotCredential); err != nil {
		t.Fatalf("json.Unmarshal(YUTU_CREDENTIAL) error = %v", err)
	}
	installed, _ := gotCredential["installed"].(map[string]any)
	if installed["client_id"] != "cid" {
		t.Fatalf("client_id = %v, want cid", installed["client_id"])
	}

	var gotToken map[string]any
	if err := json.Unmarshal([]byte(envs[1][len("YUTU_CACHE_TOKEN="):]), &gotToken); err != nil {
		t.Fatalf("json.Unmarshal(YUTU_CACHE_TOKEN) error = %v", err)
	}
	if gotToken["refresh_token"] != "ref" {
		t.Fatalf("refresh_token = %v, want ref", gotToken["refresh_token"])
	}
}

func TestBuildExecutaEnvWithAccessToken(t *testing.T) {
	t.Parallel()

	envs, err := buildExecutaEnv(map[string]any{executaAccessTokenCredential: "ya29.test-token"})
	if err != nil {
		t.Fatalf("buildExecutaEnv() error = %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("buildExecutaEnv() got %d envs, want 1", len(envs))
	}
	if envs[0][:len("YUTU_CACHE_TOKEN=")] != "YUTU_CACHE_TOKEN=" {
		t.Fatalf("buildExecutaEnv() got %q, want YUTU_CACHE_TOKEN", envs[0])
	}

	var gotToken map[string]any
	if err := json.Unmarshal([]byte(envs[0][len("YUTU_CACHE_TOKEN="):]), &gotToken); err != nil {
		t.Fatalf("json.Unmarshal(YUTU_CACHE_TOKEN) error = %v", err)
	}
	if gotToken["access_token"] != "ya29.test-token" {
		t.Fatalf("access_token = %v, want ya29.test-token", gotToken["access_token"])
	}
	if gotToken["token_type"] != "Bearer" {
		t.Fatalf("token_type = %v, want Bearer", gotToken["token_type"])
	}
}

func TestBuildExecutaEnvPrefersAccessToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	authFile := filepath.Join(dir, "authorized_user.json")
	content := `{"client_id":"cid","client_secret":"sec","refresh_token":"ref","type":"authorized_user"}`
	if err := os.WriteFile(authFile, []byte(content), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	envs, err := buildExecutaEnv(map[string]any{
		executaAccessTokenCredential:    "ya29.preferred",
		executaAuthorizedUserCredential: authFile,
	})
	if err != nil {
		t.Fatalf("buildExecutaEnv() error = %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("buildExecutaEnv() got %d envs, want 1", len(envs))
	}
	if envs[0][:len("YUTU_CACHE_TOKEN=")] != "YUTU_CACHE_TOKEN=" {
		t.Fatalf("buildExecutaEnv() got %q, want YUTU_CACHE_TOKEN", envs[0])
	}
	if bytes.Contains([]byte(envs[0]), []byte("YUTU_CREDENTIAL=")) {
		t.Fatalf("buildExecutaEnv() unexpectedly included YUTU_CREDENTIAL: %v", envs)
	}
}

func TestBuildExecutaEnvRequiresCredentials(t *testing.T) {
	t.Parallel()

	_, err := buildExecutaEnv(nil)
	if err == nil {
		t.Fatal("buildExecutaEnv() error = nil, want non-nil")
	}
	want := "missing credentials: provide GOOGLE_ACCESS_TOKEN or YUTU_AUTHORIZED_USER_FILE"
	if err.Error() != want {
		t.Fatalf("buildExecutaEnv() error = %q, want %q", err.Error(), want)
	}
}

func TestResolveExecutaWorkDir(t *testing.T) {
	t.Parallel()

	executablePath := filepath.Join(t.TempDir(), "bin", "yutu")
	if err := os.MkdirAll(filepath.Dir(executablePath), 0755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	got, err := resolveExecutaWorkDir(executablePath, map[string]any{"cwd": "results"})
	if err != nil {
		t.Fatalf("resolveExecutaWorkDir() error = %v", err)
	}

	want := filepath.Join(filepath.Dir(executablePath), "results")
	if got != want {
		t.Fatalf("resolveExecutaWorkDir() got %q, want %q", got, want)
	}
}

func TestHandleInvokeRejectsMissingCommand(t *testing.T) {
	t.Parallel()

	server := executaServer{
		executablePath: "/tmp/yutu",
		now:            time.Now,
	}

	resp := server.handleInvoke(t.Context(), executaRequest{
		JSONRPC: "2.0",
		ID:      1,
		Params: map[string]any{
			"tool":      "run_yutu",
			"arguments": map[string]any{},
		},
	})

	if resp.Error == nil {
		t.Fatal("handleInvoke() expected invalid params error")
	}
	if resp.Error.Code != -32602 {
		t.Fatalf("handleInvoke() error code = %d, want -32602", resp.Error.Code)
	}
}

func TestExecutaStderrHasCommandError(t *testing.T) {
	t.Parallel()

	if !executaStderrHasCommandError([]byte("Error: failed to create YouTube service\n")) {
		t.Fatal("executaStderrHasCommandError() = false, want true")
	}
	if executaStderrHasCommandError([]byte("🐰yutu v1.0.0 darwin/arm64\n")) {
		t.Fatal("executaStderrHasCommandError() = true, want false")
	}
}

func TestSendResponseForcesFileTransport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var stdout bytes.Buffer
	server := executaServer{stdout: &stdout}

	resp := executaResponse{
		JSONRPC:            "2.0",
		ID:                 1,
		Result:             map[string]any{"ok": true},
		forceFileTransport: true,
		fileTransportDir:   dir,
	}

	if err := server.sendResponse(resp); err != nil {
		t.Fatalf("sendResponse() error = %v", err)
	}

	var pointer map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &pointer); err != nil {
		t.Fatalf("json.Unmarshal(pointer) error = %v", err)
	}
	path, _ := pointer["__file_transport"].(string)
	if path == "" {
		t.Fatal("pointer missing __file_transport")
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("file transport dir = %q, want %q", filepath.Dir(path), dir)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	if !bytes.Contains(content, []byte(`"ok":true`)) {
		t.Fatalf("response file = %s, want marshaled result", string(content))
	}
}

func TestClassifyExecutaCommandErrorInvalidParams(t *testing.T) {
	t.Parallel()

	err := classifyExecutaCommandError(map[string]any{
		"stderr": "Error: unknown flag: --version\nUsage:\n  yutu [flags]\n",
	})
	if err.Code != -32602 {
		t.Fatalf("Code = %d, want -32602", err.Code)
	}
	if err.Message != "unknown flag: --version" {
		t.Fatalf("Message = %q, want %q", err.Message, "unknown flag: --version")
	}
}

func TestClassifyExecutaCommandErrorInternal(t *testing.T) {
	t.Parallel()

	err := classifyExecutaCommandError(map[string]any{
		"stderr": "Error: failed to create YouTube service: dial tcp: lookup oauth2.googleapis.com: no such host\n",
	})
	if err.Code != -32603 {
		t.Fatalf("Code = %d, want -32603", err.Code)
	}
}
