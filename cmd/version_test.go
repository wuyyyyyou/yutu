// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommandWritesToStdout(t *testing.T) {
	t.Parallel()

	oldVersion := Version
	oldCommitDate := CommitDate
	oldOS := Os
	oldArch := Arch
	oldBuilder := Builder
	defer func() {
		Version = oldVersion
		CommitDate = oldCommitDate
		Os = oldOS
		Arch = oldArch
		Builder = oldBuilder
	}()

	Version = "test-version"
	CommitDate = "2026-04-10T00:00:00Z"
	Os = "darwin"
	Arch = "arm64"
	Builder = "tester"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	versionCmd.SetOut(&stdout)
	versionCmd.SetErr(&stderr)
	versionCmd.Run(versionCmd, nil)

	if !strings.Contains(stdout.String(), "test-version") {
		t.Fatalf("stdout = %q, want version output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}
