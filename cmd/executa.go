// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	executaShort                    = "Run the Anna Executa plugin server over stdio"
	executaLong                     = "Run yutu as an Anna Executa plugin over stdio JSON-RPC 2.0."
	executaExample                  = "yutu executa"
	executaMaxMessageBytes          = 512 * 1024
	executaAuthorizedUserCredential = "YUTU_AUTHORIZED_USER_FILE"
	executaAccessTokenCredential    = "GOOGLE_ACCESS_TOKEN"
)

var (
	executaBlockedCommands = map[string]struct{}{
		"agent":   {},
		"auth":    {},
		"executa": {},
		"mcp":     {},
	}
	executaCmd = &cobra.Command{
		Use:     "executa",
		Short:   executaShort,
		Long:    executaLong,
		Example: executaExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			executablePath, err := os.Executable()
			if err != nil {
				return err
			}

			server := executaServer{
				executablePath: executablePath,
				stdout:         cmd.OutOrStdout(),
				stderr:         cmd.ErrOrStderr(),
				now:            time.Now,
			}
			return server.Serve(cmd.Context(), os.Stdin)
		},
	}
)

type executaServer struct {
	executablePath string
	stdout         io.Writer
	stderr         io.Writer
	now            func() time.Time
}

type executaRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
	ID      any            `json:"id"`
}

type executaResponse struct {
	JSONRPC            string           `json:"jsonrpc"`
	ID                 any              `json:"id"`
	Result             any              `json:"result,omitempty"`
	Error              *executaRPCError `json:"error,omitempty"`
	forceFileTransport bool             `json:"-"`
	fileTransportDir   string           `json:"-"`
}

type executaRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type executaFileTransport struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	File    string `json:"__file_transport"`
}

type executaInvokeResult struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data"`
	Tool    string         `json:"tool"`
}

type authorizedUserFile struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	Type         string `json:"type"`
}

func init() {
	RootCmd.AddCommand(executaCmd)
}

func ExecuteExecuta(ctx context.Context, executablePath string, in io.Reader, stdout, stderr io.Writer) error {
	server := executaServer{
		executablePath: executablePath,
		stdout:         stdout,
		stderr:         stderr,
		now:            time.Now,
	}
	return server.Serve(ctx, in)
}

func (s executaServer) Serve(ctx context.Context, in io.Reader) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		resp := s.handleLine(ctx, line)
		if err := s.sendResponse(resp); err != nil {
			fmt.Fprintf(s.stderr, "failed to send response: %v\n", err)
		}
	}

	return scanner.Err()
}

func (s executaServer) handleLine(ctx context.Context, line string) executaResponse {
	var req executaRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return executaResponse{
			JSONRPC: "2.0",
			Error: &executaRPCError{
				Code:    -32700,
				Message: "Parse error",
			},
		}
	}

	switch req.Method {
	case "describe":
		return executaResponse{JSONRPC: "2.0", ID: req.ID, Result: s.manifest()}
	case "health":
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"status":      "healthy",
				"timestamp":   s.now().UTC().Format(time.RFC3339),
				"version":     s.version(),
				"tools_count": 1,
			},
		}
	case "invoke":
		return s.handleInvoke(ctx, req)
	default:
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
		}
	}
}

func (s executaServer) manifest() map[string]any {
	return map[string]any{
		"name":         "yutu-executa",
		"display_name": "Yutu",
		"version":      s.version(),
		"description":  "Run yutu CLI commands from Anna through a single run_yutu tool.",
		"author":       "eat-pray-ai & OpenWaygate",
		"credentials": []map[string]any{
			{
				"name":         executaAccessTokenCredential,
				"display_name": "Google Access Token",
				"description":  "OAuth access token for direct YouTube API calls. When present, it is used before YUTU_AUTHORIZED_USER_FILE.",
				"required":     false,
				"sensitive":    true,
			},
			{
				"name":         executaAuthorizedUserCredential,
				"display_name": "Authorized User JSON Path",
				"description":  "Absolute path to a local Google authorized_user JSON file containing client_id, client_secret, and refresh_token.",
				"required":     false,
				"sensitive":    false,
			},
		},
		"tools": []map[string]any{
			{
				"name":        "run_yutu",
				"description": "Run a yutu CLI command. command is an argv array, auth/agent/mcp/executa are blocked, commands with an output flag default to --output json, and the invoke response is always returned through standard Executa __file_transport so Anna can ingest large outputs safely.",
				"parameters": []map[string]any{
					{
						"name":        "command",
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "The yutu command argv to run, for example [\"search\", \"list\", \"--q\", \"golang\"] or [\"yutu\", \"search\", \"list\", \"--q\", \"golang\"].",
						"required":    true,
					},
					{
						"name":        "cwd",
						"type":        "string",
						"description": "Working directory for the command and output file. When omitted, the plugin binary directory is used.",
						"required":    false,
					},
				},
			},
		},
		"runtime": map[string]any{
			"type":        "binary",
			"min_version": "1.0.0",
		},
	}
}

func (s executaServer) handleInvoke(ctx context.Context, req executaRequest) executaResponse {
	toolName, _ := req.Params["tool"].(string)
	if toolName == "" {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32602,
				Message: "Missing 'tool' in params",
			},
		}
	}
	if toolName != "run_yutu" {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32601,
				Message: fmt.Sprintf("Unknown tool: %s", toolName),
				Data:    map[string]any{"available_tools": []string{"run_yutu"}},
			},
		}
	}

	args, err := parseCommandArray(req.Params["arguments"])
	if err != nil {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32602,
				Message: err.Error(),
			},
		}
	}

	arguments, _ := req.Params["arguments"].(map[string]any)
	workDir, err := resolveExecutaWorkDir(s.executablePath, arguments)
	if err != nil {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32602,
				Message: err.Error(),
			},
		}
	}

	normalizedArgs, err := normalizeExecutaCommand(s.executablePath, args)
	if err != nil {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32602,
				Message: err.Error(),
			},
		}
	}

	credentials := extractExecutaCredentials(req.Params)
	envAdditions, err := buildExecutaEnv(credentials)
	if err != nil {
		return executaResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &executaRPCError{
				Code:    -32602,
				Message: err.Error(),
			},
		}
	}

	data, success, err := s.runYutu(ctx, workDir, normalizedArgs, envAdditions)
	if err != nil {
		return executaResponse{
			JSONRPC:            "2.0",
			ID:                 req.ID,
			forceFileTransport: true,
			fileTransportDir:   workDir,
			Error: &executaRPCError{
				Code:    -32603,
				Message: fmt.Sprintf("Failed to execute yutu: %v", err),
			},
		}
	}
	if !success {
		return executaResponse{
			JSONRPC:            "2.0",
			ID:                 req.ID,
			forceFileTransport: true,
			fileTransportDir:   workDir,
			Error:              classifyExecutaCommandError(data),
		}
	}

	return executaResponse{
		JSONRPC:            "2.0",
		ID:                 req.ID,
		forceFileTransport: true,
		fileTransportDir:   workDir,
		Result: executaInvokeResult{
			Success: success,
			Data:    data,
			Tool:    toolName,
		},
	}
}

func parseCommandArray(raw any) ([]string, error) {
	arguments, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("Missing 'arguments' in params")
	}

	commandRaw, ok := arguments["command"].([]any)
	if !ok {
		return nil, errors.New("Missing required parameter: command")
	}

	command := make([]string, 0, len(commandRaw))
	for _, item := range commandRaw {
		part, ok := item.(string)
		if !ok {
			return nil, errors.New("Parameter 'command' must be an array of strings")
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		command = append(command, part)
	}

	if len(command) == 0 {
		return nil, errors.New("Parameter 'command' must not be empty")
	}

	return command, nil
}

func resolveExecutaWorkDir(executablePath string, arguments map[string]any) (string, error) {
	baseDir := filepath.Dir(executablePath)
	cwd, _ := arguments["cwd"].(string)
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		if err := os.MkdirAll(baseDir, 0755); err != nil {
			return "", fmt.Errorf("failed to prepare default cwd: %w", err)
		}
		return baseDir, nil
	}

	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(baseDir, cwd)
	}
	cwd = filepath.Clean(cwd)

	if err := os.MkdirAll(cwd, 0755); err != nil {
		return "", fmt.Errorf("failed to prepare cwd %q: %w", cwd, err)
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to stat cwd %q: %w", cwd, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd %q is not a directory", cwd)
	}

	return cwd, nil
}

func normalizeExecutaCommand(executablePath string, args []string) ([]string, error) {
	cleaned := slices.Clone(args)
	if len(cleaned) == 0 {
		return nil, errors.New("Parameter 'command' must not be empty")
	}

	firstName := stripExecutableExt(filepath.Base(cleaned[0]))
	selfName := stripExecutableExt(filepath.Base(executablePath))
	if firstName == "yutu" || firstName == selfName {
		cleaned = cleaned[1:]
	}
	if len(cleaned) == 0 {
		return nil, errors.New("Parameter 'command' must include a yutu subcommand")
	}

	if _, blocked := executaBlockedCommands[cleaned[0]]; blocked {
		return nil, fmt.Errorf("Subcommand %q is not allowed in run_yutu", cleaned[0])
	}

	return ensureJSONOutput(cleaned), nil
}

func ensureJSONOutput(args []string) []string {
	if hasOutputFlag(args) {
		return args
	}

	target, _, err := RootCmd.Find(args)
	if err != nil || target == nil || target == RootCmd {
		return args
	}
	if target.Flags().Lookup("output") == nil {
		return args
	}

	withOutput := make([]string, 0, len(args)+2)
	withOutput = append(withOutput, args...)
	withOutput = append(withOutput, "--output", "json")
	return withOutput
}

func hasOutputFlag(args []string) bool {
	for i, arg := range args {
		if arg == "--output" || strings.HasPrefix(arg, "--output=") {
			return true
		}
		if arg == "-o" && i < len(args)-1 {
			return true
		}
		if strings.HasPrefix(arg, "-o=") {
			return true
		}
	}
	return false
}

func extractExecutaCredentials(params map[string]any) map[string]any {
	contextMap, _ := params["context"].(map[string]any)
	credentials, _ := contextMap["credentials"].(map[string]any)
	return credentials
}

func buildExecutaEnv(credentials map[string]any) ([]string, error) {
	accessToken := lookupCredentialValue(credentials, executaAccessTokenCredential)
	if accessToken == "" {
		accessToken = strings.TrimSpace(os.Getenv(executaAccessTokenCredential))
	}
	if accessToken != "" {
		cacheTokenJSON, err := json.Marshal(map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to build YUTU_CACHE_TOKEN from %s: %w", executaAccessTokenCredential, err)
		}

		return []string{
			"YUTU_CACHE_TOKEN=" + string(cacheTokenJSON),
		}, nil
	}

	path := lookupCredentialValue(credentials, executaAuthorizedUserCredential)
	if path == "" {
		path = os.Getenv(executaAuthorizedUserCredential)
	}
	if path == "" {
		return nil, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", executaAuthorizedUserCredential, err)
	}

	var credential authorizedUserFile
	if err := json.Unmarshal(content, &credential); err != nil {
		return nil, fmt.Errorf("failed to parse authorized_user JSON: %w", err)
	}
	if credential.Type != "authorized_user" {
		return nil, fmt.Errorf("unsupported credential type %q in %s", credential.Type, path)
	}
	if credential.ClientID == "" || credential.ClientSecret == "" || credential.RefreshToken == "" {
		return nil, fmt.Errorf("%s must contain client_id, client_secret, and refresh_token", path)
	}

	clientSecretJSON, err := json.Marshal(map[string]any{
		"installed": map[string]any{
			"client_id":                   credential.ClientID,
			"project_id":                  "anna-yutu-executa",
			"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
			"token_uri":                   "https://oauth2.googleapis.com/token",
			"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
			"client_secret":               credential.ClientSecret,
			"redirect_uris":               []string{"http://localhost:8216"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build YUTU_CREDENTIAL: %w", err)
	}

	cacheTokenJSON, err := json.Marshal(map[string]any{
		"refresh_token": credential.RefreshToken,
		"token_type":    "Bearer",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build YUTU_CACHE_TOKEN: %w", err)
	}

	return []string{
		"YUTU_CREDENTIAL=" + string(clientSecretJSON),
		"YUTU_CACHE_TOKEN=" + string(cacheTokenJSON),
	}, nil
}

func lookupCredentialValue(credentials map[string]any, key string) string {
	if credentials == nil {
		return ""
	}
	value, _ := credentials[key].(string)
	return strings.TrimSpace(value)
}

func (s executaServer) runYutu(
	ctx context.Context, workDir string, args []string, envAdditions []string,
) (map[string]any, bool, error) {
	outputFile, err := os.CreateTemp(workDir, "yutu-run-stdout-*.tmp")
	if err != nil {
		return nil, false, fmt.Errorf("failed to create stdout capture file: %w", err)
	}
	outputPath := outputFile.Name()

	var stderr bytes.Buffer
	command := exec.CommandContext(ctx, s.executablePath, args...)
	command.Dir = workDir
	command.Stdout = outputFile
	command.Stderr = &stderr
	command.Env = mergeExecutaEnv(os.Environ(), envAdditions...)
	command.Env = mergeExecutaEnv(command.Env, "YUTU_ROOT="+workDir)

	start := s.now()
	runErr := command.Run()
	duration := s.now().Sub(start)

	closeErr := outputFile.Close()
	if closeErr != nil {
		return nil, false, fmt.Errorf("failed to close stdout capture file: %w", closeErr)
	}
	defer func() {
		_ = os.Remove(outputPath)
	}()

	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			_ = os.Remove(outputPath)
			return nil, false, runErr
		}
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to stat stdout capture file: %w", err)
	}
	stdoutBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to read stdout capture file: %w", err)
	}

	exitCode := 0
	success := runErr == nil
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		success = false
	}
	if executaStderrHasCommandError(stderr.Bytes()) {
		success = false
	}

	data := map[string]any{
		"command":      args,
		"cwd":          workDir,
		"output_bytes": info.Size(),
		"exit_code":    exitCode,
		"duration_ms":  duration.Milliseconds(),
	}
	if len(stdoutBytes) > 0 {
		trimmed := bytes.TrimSpace(stdoutBytes)
		if json.Valid(trimmed) {
			data["output"] = json.RawMessage(trimmed)
		} else {
			data["output"] = string(stdoutBytes)
		}
	}
	if stderr.Len() > 0 {
		data["stderr"] = stderr.String()
	}

	return data, success, nil
}

func (s executaServer) sendResponse(resp executaResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	if resp.forceFileTransport || len(payload) > executaMaxMessageBytes {
		dir := resp.fileTransportDir
		if strings.TrimSpace(dir) == "" {
			dir = ""
		}
		file, err := os.CreateTemp(dir, "executa-resp-*.json")
		if err != nil {
			return err
		}

		if _, err := file.Write(payload); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}

		pointer, err := json.Marshal(executaFileTransport{
			JSONRPC: "2.0",
			ID:      resp.ID,
			File:    file.Name(),
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(s.stdout, string(pointer))
		return err
	}

	_, err = fmt.Fprintln(s.stdout, string(payload))
	return err
}

func (s executaServer) version() string {
	if Version != "" {
		return Version
	}

	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func stripExecutableExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func executaStderrHasCommandError(stderr []byte) bool {
	for _, line := range strings.Split(string(stderr), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Error:") {
			return true
		}
	}
	return false
}

func classifyExecutaCommandError(data map[string]any) *executaRPCError {
	stderr, _ := data["stderr"].(string)
	message := firstExecutaErrorLine(stderr)
	if message == "" {
		message = "yutu command failed"
	}

	code := -32603
	if executaLooksLikeInvalidParams(stderr) {
		code = -32602
	}

	return &executaRPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

func firstExecutaErrorLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Error:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Error:"))
		}
	}
	return ""
}

func executaLooksLikeInvalidParams(stderr string) bool {
	lower := strings.ToLower(stderr)
	patterns := []string{
		"error: unknown flag:",
		"error: unknown command",
		"error: accepts ",
		"error: required flag",
		"error: invalid argument",
		"error: invalid value",
		"error: argument ",
		"error: flag needs",
		"error: requires at least",
		"error: requires at most",
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func mergeExecutaEnv(base []string, additions ...string) []string {
	merged := slices.Clone(base)
	for _, addition := range additions {
		key, _, ok := strings.Cut(addition, "=")
		if !ok {
			merged = append(merged, addition)
			continue
		}

		replaced := false
		for i, existing := range merged {
			existingKey, _, existingOK := strings.Cut(existing, "=")
			if existingOK && existingKey == key {
				merged[i] = addition
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, addition)
		}
	}
	return merged
}
