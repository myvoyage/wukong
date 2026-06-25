// Package zim provides ZIM file format creation and reading.
package zim

import (
	"os"
	"testing"
)

// TestZIMRoundtrip verifies the complete write-then-read cycle.
func TestZIMRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test.zim"

	// 1. Build ZIM archive.
	p := NewPacker()
	p.AddContent('C', "pages/index.html", "Home", "text/html",
		[]byte("<html><body>Hello World</body></html>"))
	p.AddContent('C', "pages/about/index.html", "About", "text/html",
		[]byte("<html><body>About Us</body></html>"))
	p.AddContent('C', "assets/style.css", "styles", "text/css",
		[]byte("body{color:red;}"))
	p.AddContent('C', "assets/logo.png", "logo", "image/png",
		[]byte{0x89, 0x50, 0x4E, 0x47, 0, 0, 0, 0}) // minimal PNG header
	p.AddMetadata("Title", "Test Archive")
	p.AddMetadata("Language", "eng")
	p.AddMetadata("Date", "2026-06-25")
	p.SetMainPage('C', "pages/index.html")
	p.AddRedirect('W', "mainPage", "Main Page", 'C', "pages/index.html")

	err := p.Build(zimPath, "TestApp", "roundtrip test", true)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 2. Read ZIM archive.
	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if r.Count() != 9 { // 4 content + 3 metadata + 1 redirect + 1 W/mainPage redirect
		t.Errorf("Count = %d, want 9", r.Count())
	}

	// 3. Verify content articles via Get.
	tests := []struct {
		ns   byte
		url  string
		want string
	}{
		{'C', "pages/index.html", "<html><body>Hello World</body></html>"},
		{'C', "pages/about/index.html", "<html><body>About Us</body></html>"},
		{'C', "assets/style.css", "body{color:red;}"},
	}

	for _, tt := range tests {
		blob, err := r.Get(tt.ns, tt.url)
		if err != nil {
			t.Errorf("Get(%c/%s): %v", tt.ns, tt.url, err)
			continue
		}
		if string(blob.Data) != tt.want {
			t.Errorf("Get(%c/%s) = %q, want %q",
				tt.ns, tt.url, string(blob.Data), tt.want)
		}
	}

	// 4. Verify metadata.
	metaTests := []struct{ name, want string }{
		{"Title", "Test Archive"},
		{"Language", "eng"},
		{"Date", "2026-06-25"},
	}
	for _, mt := range metaTests {
		blob, err := r.Get('M', mt.name)
		if err != nil {
			t.Errorf("Get(M/%s): %v", mt.name, err)
			continue
		}
		if string(blob.Data) != mt.want {
			t.Errorf("Get(M/%s) = %q, want %q",
				mt.name, string(blob.Data), mt.want)
		}
	}

	// 5. Verify W/mainPage redirect resolves to content.
	blob, err := r.Get('W', "mainPage")
	if err != nil {
		t.Fatalf("Get(W/mainPage): %v", err)
	}
	if blob.URL != "pages/index.html" {
		t.Errorf("mainPage redirect = %q, want %q",
			blob.URL, "pages/index.html")
	}
	if string(blob.Data) != "<html><body>Hello World</body></html>" {
		t.Errorf("mainPage content = %q", string(blob.Data))
	}

	// 6. Verify image binary content roundtrip.
	pngBlob, err := r.Get('C', "assets/logo.png")
	if err != nil {
		t.Errorf("Get(C/assets/logo.png): %v", err)
	} else if len(pngBlob.Data) < 8 {
		t.Errorf("PNG data too short: %d bytes", len(pngBlob.Data))
	}
}

// TestZIMRoundtrip_NoCompress verifies uncompressed archives.
func TestZIMRoundtrip_NoCompress(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test_noz.zim"

	p := NewPacker()
	p.AddContent('C', "index.html", "Home", "text/html",
		[]byte("<h1>Uncompressed</h1>"))
	p.Build(zimPath, "Test", "no compress", false)

	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	blob, err := r.Get('C', "index.html")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(blob.Data) != "<h1>Uncompressed</h1>" {
		t.Errorf("got %q", string(blob.Data))
	}

	// Verify file was actually written.
	fi, err := os.Stat(zimPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("ZIM file is empty")
	}
}

// TestZIMRedirect verifies redirect resolution.
func TestZIMRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test_redir.zim"

	p := NewPacker()
	p.AddContent('C', "page2.html", "Page 2", "text/html",
		[]byte("page two"))
	p.AddRedirect('C', "old.html", "Old Page", 'C', "page2.html")
	p.Build(zimPath, "Test", "redirect", false)

	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	// Following redirect should return target content.
	blob, err := r.Get('C', "old.html")
	if err != nil {
		t.Fatalf("Get(C/old.html): %v", err)
	}
	if string(blob.Data) != "page two" {
		t.Errorf("redirected content = %q, want %q",
			string(blob.Data), "page two")
	}
	if blob.URL != "page2.html" {
		t.Errorf("redirect URL = %q, want %q",
			blob.URL, "page2.html")
	}
}

// TestZIMCount verifies article count and entry iteration.
func TestZIMCount(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test_count.zim"

	p := NewPacker()
	for i := 0; i < 10; i++ {
		url := string(rune('a'+i)) + ".html"
		p.AddContent('C', url, url, "text/html", []byte(url))
	}
	p.Build(zimPath, "Count", "count test", false)

	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	if r.Count() != 10 {
		t.Errorf("Count = %d, want 10", r.Count())
	}

	// Verify URL-sorted order.
	for i := uint32(0); i < r.Count(); i++ {
		entry, err := r.EntryAt(i)
		if err != nil {
			t.Errorf("EntryAt(%d): %v", i, err)
			continue
		}
		want := string(rune('a'+i)) + ".html"
		if entry.URL != want {
			t.Errorf("EntryAt(%d) URL = %q, want %q", i, entry.URL, want)
		}
	}
}

// TestZIMIncrementalCache verifies cluster re-use on rebuild.
func TestZIMIncrementalCache(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test.zim"
	cachePath := tmpDir + "/test.wukongcache"

	p := NewPacker()
	p.AddContent('C', "index.html", "Home", "text/html",
		[]byte("<h1>Hello</h1>"))
	p.AddContent('C', "style.css", "CSS", "text/css",
		[]byte("body{margin:0;}"))

	// First build (should compress all clusters).
	stats, err := p.BuildWithStats(zimPath, BuildOptions{
		AppName:  "Test",
		Compress: true,
	}, cachePath, true)
	if err != nil {
		t.Fatalf("BuildWithStats: %v", err)
	}
	if stats.ClustersReused != 0 {
		t.Errorf("first build reused = %d, want 0", stats.ClustersReused)
	}

	// Rebuild with same content (should reuse all clusters).
	p2 := NewPacker()
	p2.AddContent('C', "index.html", "Home", "text/html",
		[]byte("<h1>Hello</h1>"))
	p2.AddContent('C', "style.css", "CSS", "text/css",
		[]byte("body{margin:0;}"))

	stats2, err := p2.BuildWithStats(zimPath, BuildOptions{
		AppName:  "Test",
		Compress: true,
	}, cachePath, true)
	if err != nil {
		t.Fatalf("BuildWithStats 2: %v", err)
	}
	if stats2.ClustersReused == 0 {
		t.Errorf("second build reused = %d, want > 0", stats2.ClustersReused)
	}

	// Verify content still readable.
	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	blob, _ := r.Get('C', "index.html")
	if string(blob.Data) != "<h1>Hello</h1>" {
		t.Errorf("content = %q", string(blob.Data))
	}
}

// TestZIMEmptyArticles verifies empty articles and edge cases.
func TestZIMEmptyArticles(t *testing.T) {
	tmpDir := t.TempDir()
	zimPath := tmpDir + "/test_empty.zim"

	p := NewPacker()
	p.AddContent('C', "empty.html", "Empty", "text/html", []byte{})
	p.AddMetadata("Description", "")
	p.Build(zimPath, "Empty", "test", false)

	r, err := Open(zimPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	blob, err := r.Get('C', "empty.html")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(blob.Data) != 0 {
		t.Errorf("empty article data = %d bytes, want 0", len(blob.Data))
	}

	meta, err := r.Get('M', "Description")
	if err != nil {
		t.Fatalf("Get(M/Description): %v", err)
	}
	if len(meta.Data) != 0 {
		t.Errorf("empty metadata = %d bytes, want 0", len(meta.Data))
	}
}
