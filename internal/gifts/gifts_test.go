package gifts

import (
	"regexp"
	"testing"
)

func TestRandomColorIsValidAndReadable(t *testing.T) {
	re := regexp.MustCompile(`^#[0-9a-f]{6}$`)
	for i := 0; i < 500; i++ {
		bg, text := randomColor()
		if !re.MatchString(bg) {
			t.Fatalf("bad colour %q", bg)
		}
		if text != "#000000" && text != "#ffffff" {
			t.Fatalf("bad text colour %q", text)
		}
	}
}

func TestTextColor(t *testing.T) {
	if got := textFor(0, 0, 0); got != "#ffffff" {
		t.Fatalf("black background -> %s, want white text", got)
	}
	if got := textFor(255, 255, 255); got != "#000000" {
		t.Fatalf("white background -> %s, want black text", got)
	}
}

func TestInputClean(t *testing.T) {
	ok := Input{Title: " Book ", URL: "https://example.com/x", Tags: []string{"A", "a", " ", "B"}}
	if err := ok.clean(); err != nil || ok.Title != "Book" || len(ok.Tags) != 2 {
		t.Fatalf("clean = %v %+v", err, ok)
	}
	for _, bad := range []Input{
		{Title: ""},
		{Title: "x", URL: "javascript:alert(1)"},
		{Title: "x", URL: "//example.com"},
		{Title: "x", Tags: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}},
	} {
		if err := bad.clean(); err == nil {
			t.Fatalf("%+v must be rejected", bad)
		}
	}
}
