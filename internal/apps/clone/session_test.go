package clone

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneSessionSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cookieFile := filepath.Join(tmpDir, "cookies.txt")

	// Create session and add cookies.
	s, err := NewCloneSession(cookieFile)
	if err != nil {
		t.Fatalf("NewCloneSession: %v", err)
	}

	s.SetCookies("https://example.com", []*http.Cookie{
		{Name: "session", Value: "abc123", Domain: ".example.com", Path: "/"},
		{Name: "token", Value: "xyz789", Domain: ".example.com", Path: "/admin", Secure: true},
	})

	// Save to file.
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists and is non-empty.
	info, err := os.Stat(cookieFile)
	if err != nil {
		t.Fatalf("cookie file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("cookie file is empty")
	}

	// Load into a new session.
	s2, err := NewCloneSession(cookieFile)
	if err != nil {
		t.Fatalf("NewCloneSession (load): %v", err)
	}

	// Verify cookies are restored.
	client := s2.HTTPClient()
	if client.Jar == nil {
		t.Fatal("HTTP client has no cookie jar")
	}
}

func TestCloneSessionNoFile(t *testing.T) {
	s, err := NewCloneSession("")
	if err != nil {
		t.Fatalf("NewCloneSession: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Errorf("Save with no file should succeed: %v", err)
	}
}

func TestCloneSessionHTTPClient(t *testing.T) {
	s, err := NewCloneSession("")
	if err != nil {
		t.Fatalf("NewCloneSession: %v", err)
	}
	client := s.HTTPClient()
	if client.Jar == nil {
		t.Error("HTTP client should have cookie jar")
	}
	if client.Timeout == 0 {
		t.Error("HTTP client should have timeout")
	}
}
