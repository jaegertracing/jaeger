// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"os"
	"testing"
	"time"
)

func TestCachedFileTokenLoader_Basic(t *testing.T) {
	tmpfile, err := os.CreateTemp(t.TempDir(), "tokenfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	token := "my-secret-token"
	if _, err := tmpfile.WriteString(token + "\n"); err != nil {
		t.Fatalf("failed to write token: %v", err)
	}
	tmpfile.Close()

	loader := CachedFileTokenLoader(tmpfile.Name(), 100*time.Millisecond)

	// First load - should read from file
	t1, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t1 != token {
		t.Errorf("expected %q, got %q", token, t1)
	}

	// Change the file, but within cache interval
	os.WriteFile(tmpfile.Name(), []byte("new-token\n"), 0o644)
	t2, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t2 != token {
		t.Errorf("expected cached %q, got %q", token, t2)
	}

	// Wait for cache to expire
	time.Sleep(120 * time.Millisecond)
	t3, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t3 != "new-token" {
		t.Errorf("expected refreshed token 'new-token', got %q", t3)
	}
}
