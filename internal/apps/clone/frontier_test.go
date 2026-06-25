// Package clone provides website cloning functionality.
package clone

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontierOffer(t *testing.T) {
	f := newFrontier()

	if !f.offer("key1") {
		t.Error("first offer should accept")
	}
	if f.offer("key1") {
		t.Error("duplicate offer should reject")
	}
	if !f.offer("key2") {
		t.Error("new key should accept")
	}
}

func TestFrontierMarkVisited(t *testing.T) {
	f := newFrontier()

	f.markVisited("key1")
	if !f.isVisited("key1") {
		t.Error("key1 should be visited")
	}
	if f.isVisited("key2") {
		t.Error("key2 should not be visited")
	}
	if f.visitedCount() != 1 {
		t.Errorf("visitedCount = %d, want 1", f.visitedCount())
	}
}

func TestFrontierSaveLoad(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "_wukong", "state.json")

	f1 := newFrontier()
	f1.markVisited("page_a")
	f1.markVisited("page_b")
	f1.markVisited("page_c")
	f1.offer("page_d") // Seen but not visited.

	if err := f1.save(statePath); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load into a new frontier.
	f2 := newFrontier()
	if err := f2.load(statePath); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !f2.isVisited("page_a") {
		t.Error("loaded frontier should have page_a visited")
	}
	if !f2.isVisited("page_b") {
		t.Error("loaded frontier should have page_b visited")
	}
	if !f2.isVisited("page_c") {
		t.Error("loaded frontier should have page_c visited")
	}
	// page_d was seen but not visited, should not be in loaded state.
	if f2.isVisited("page_d") {
		t.Error("page_d should not be in visited set (was only seen)")
	}
	// However, loaded visited URLs should also be marked as seen.
	if f2.offer("page_a") {
		t.Error("page_a should be seen in loaded frontier")
	}
}

func TestFrontierLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "nonexistent", "state.json")

	f := newFrontier()
	if err := f.load(statePath); err != nil {
		t.Fatalf("load non-existent should not error: %v", err)
	}
	if f.visitedCount() != 0 {
		t.Error("loading non-existent file should result in zero visited")
	}
}

func TestFrontierConcurrent(t *testing.T) {
	f := newFrontier()
	done := make(chan bool)

	for i := 0; i < 100; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + id%26))
				f.offer(key)
				f.markVisited(key)
				f.visitedCount()
			}
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}
	// No panic means the test passes for concurrency.
}

func TestFrontierSaveFileContent(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	f := newFrontier()
	f.markVisited("zzz")
	f.markVisited("aaa")
	f.markVisited("mmm")

	if err := f.save(statePath); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify sorted order in file.
	content := string(data)
	if !containsOrder(content, "aaa", "mmm", "zzz") {
		t.Errorf("visited keys should be sorted, got: %s", content)
	}
}

func containsOrder(s, a, b, c string) bool {
	ia := indexOf(s, a)
	ib := indexOf(s, b)
	ic := indexOf(s, c)
	return ia >= 0 && ib >= 0 && ic >= 0 && ia < ib && ib < ic
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
