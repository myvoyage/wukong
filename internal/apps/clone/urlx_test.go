package clone

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		ref     string
		want    string
		wantErr bool
	}{
		{
			name: "absolute same origin",
			base: "https://example.com/",
			ref:  "https://example.com/page",
			want: "https://example.com/page",
		},
		{
			name: "relative path",
			base: "https://example.com/dir/",
			ref:  "other.html",
			want: "https://example.com/dir/other.html",
		},
		{
			name: "remove fragment",
			base: "https://example.com/",
			ref:  "https://example.com/page#section",
			want: "https://example.com/page",
		},
		{
			name: "remove default port 80",
			base: "https://example.com/",
			ref:  "http://example.com:80/page",
			want: "http://example.com/page",
		},
		{
			name: "remove default port 443",
			base: "https://example.com/",
			ref:  "https://example.com:443/page",
			want: "https://example.com/page",
		},
		{
			name:    "reject javascript",
			base:    "https://example.com/",
			ref:     "javascript:void(0)",
			wantErr: true,
		},
		{
			name:    "reject mailto",
			base:    "https://example.com/",
			ref:     "mailto:test@example.com",
			wantErr: true,
		},
		{
			name:    "reject data",
			base:    "https://example.com/",
			ref:     "data:text/plain,hello",
			wantErr: true,
		},
		{
			name: "clean path dots",
			base: "https://example.com/dir/",
			ref:  "../page",
			want: "https://example.com/page",
		},
		{
			name: "trailing slash preserve",
			base: "https://example.com/",
			ref:  "https://example.com/docs/",
			want: "https://example.com/docs/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.base, tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Normalize() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLocalPath_Page(t *testing.T) {
	seedHost := "example.com"

	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/", "index.html"},
		{"https://example.com/about/", "about/index.html"},
		{"https://example.com/about/team.html", "about/team.html"},
		{"https://example.com/about/team", "about/team.html"},
		{"https://sub.example.com/page", "sub.example.com/page.html"},
		{"https://sub.example.com/page/", "sub.example.com/page/index.html"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := LocalPath(seedHost, tt.url, KindPage)
			// Use forward slash for cross-platform testing.
			got = strings.ReplaceAll(got, "\\", "/")
			if got != tt.want {
				t.Errorf("LocalPath(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestLocalPath_Asset(t *testing.T) {
	tests := []struct {
		url  string
		wantPrefix string
	}{
		{
			url:  "https://example.com/css/style.css",
			wantPrefix: "_wukong/example.com/css/style.css",
		},
		{
			url:  "https://cdn.example.com/img/logo.png",
			wantPrefix: "_wukong/cdn.example.com/img/logo.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := LocalPath("", tt.url, KindAsset)
			got = strings.ReplaceAll(got, "\\", "/")
			if got != tt.wantPrefix {
				t.Errorf("LocalPath(%q) = %q, want %q", tt.url, got, tt.wantPrefix)
			}
		})
	}
}

func TestLikelyPage(t *testing.T) {
	pages := []string{"/", "/about", "/docs/", "/index.html"}
	assets := []string{"/style.css", "/img.png", "/doc.pdf", "/data.json"}

	for _, p := range pages {
		if !LikelyPage(p) {
			t.Errorf("LikelyPage(%q) = false, want true", p)
		}
	}
	for _, a := range assets {
		if LikelyPage(a) {
			t.Errorf("LikelyPage(%q) = true, want false", a)
		}
	}
}

func TestRel(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want string
	}{
		{"pages/index.html", "pages/about.html", "about.html"},
		{"pages/about/index.html", "pages/index.html", "../index.html"},
		{"pages/docs/index.html", "pages/docs/guide.html", "guide.html"},
		// from "pages/a/b/c.html" up to "pages/index.html" = 2 levels up
		{"pages/a/b/c.html", "pages/index.html", "../../index.html"},
		// same file: fromDir="pages", toFile="pages/index.html" → "index.html"
		{"pages/index.html", "pages/index.html", "index.html"},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_"+tt.to, func(t *testing.T) {
			got := Rel(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("Rel(%q, %q) = %q, want %q",
					tt.from, tt.to, got, tt.want)
			}
		})
	}
}
