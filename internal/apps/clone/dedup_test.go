package clone

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContentDeduper_TryDedup(t *testing.T) {
	dir := t.TempDir()

	cd := NewContentDeduper(false)
	content1 := []byte("hello world")
	content2 := []byte("different content")

	// First write: should not dedup.
	path1 := filepath.Join(dir, "file1.html")
	deduped, err := cd.TryDedup(content1, path1)
	if err != nil {
		t.Fatalf("TryDedup: %v", err)
	}
	if deduped {
		t.Error("first write should not dedup")
	}

	// Write the file so it exists for hard link.
	if err := os.WriteFile(path1, content1, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cd.MarkWritten(content1, path1)

	// Second write with same content: should dedup.
	path2 := filepath.Join(dir, "file2.html")
	deduped, err = cd.TryDedup(content1, path2)
	if err != nil {
		t.Fatalf("TryDedup 2: %v", err)
	}
	if !deduped {
		t.Error("second write with same content should dedup")
	}

	// Verify hard link was created.
	fi, err := os.Stat(path2)
	if err != nil {
		t.Fatalf("Stat path2: %v", err)
	}
	if fi.Size() != int64(len(content1)) {
		t.Errorf("hard linked file size = %d, want %d", fi.Size(), len(content1))
	}

	// Different content: should not dedup.
	path3 := filepath.Join(dir, "file3.html")
	deduped, err = cd.TryDedup(content2, path3)
	if err != nil {
		t.Fatalf("TryDedup 3: %v", err)
	}
	if deduped {
		t.Error("different content should not dedup")
	}
}

func TestContentDeduper_Savings(t *testing.T) {
	dir := t.TempDir()

	cd := NewContentDeduper(false)

	// Write initial file.
	content := []byte("repeated content here")
	path1 := filepath.Join(dir, "base.html")
	os.WriteFile(path1, content, 0644)
	cd.MarkWritten(content, path1)

	// Dedup 3 more files.
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "dup"+string(rune('a'+i))+".html")
		cd.TryDedup(content, path)
	}

	files, bytes := cd.Savings()
	if files != 3 {
		t.Errorf("savings files = %d, want 3", files)
	}
	expectedBytes := int64(3 * len(content))
	if bytes != expectedBytes {
		t.Errorf("savings bytes = %d, want %d", bytes, expectedBytes)
	}
}

func TestContentDeduper_Disabled(t *testing.T) {
	cd := NewContentDeduper(true)

	deduped, err := cd.TryDedup([]byte("test"), "/tmp/test.html")
	if err != nil {
		t.Fatalf("TryDedup on disabled: %v", err)
	}
	if deduped {
		t.Error("disabled deduper should never dedup")
	}

	files, bytes := cd.Savings()
	if files != 0 || bytes != 0 {
		t.Error("disabled deduper should have zero savings")
	}

	stats := cd.Stats()
	if stats != "dedup: disabled" {
		t.Errorf("Stats() = %q, want 'dedup: disabled'", stats)
	}
}

func TestContentDeduper_Reset(t *testing.T) {
	dir := t.TempDir()
	cd := NewContentDeduper(false)

	content := []byte("reset test")
	path := filepath.Join(dir, "file.html")
	os.WriteFile(path, content, 0644)
	cd.MarkWritten(content, path)

	cd.TryDedup(content, path+"2")

	if f, _ := cd.Savings(); f != 1 {
		t.Fatalf("expected 1 dedup file, got %d", f)
	}

	cd.Reset()
	if f, _ := cd.Savings(); f != 0 {
		t.Error("after reset, savings should be zero")
	}
}

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := humanBytes(tt.n)
		if got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
