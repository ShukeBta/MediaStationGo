package service

import (
	"strings"
	"testing"
)

func TestApplyRecognitionWordRules(t *testing.T) {
	rules := parseRecognitionWordRules(`
BADWORD
Wrong.Title => 正确标题
One\.Piece\.S01E(89[2-9]|9\d{2}|10\d{2})\.1999 => 海贼王.S21E\1.1999 && S21E <> \. >> EP-892
`)
	got := applyRecognitionWordRules("BADWORD Wrong.Title One.Piece.S01E1076.1999.1080p", rules)
	want := "正确标题 海贼王.S21E184.1999.1080p"
	if got != want {
		t.Fatalf("recognized = %q, want %q", got, want)
	}
}

func TestCleanQueryWithRecognitionDisabledByDefaultRepoNil(t *testing.T) {
	title, year := CleanQueryWithRecognition(t.Context(), nil, "Dune.2021.2160p.WEB-DL.mkv")
	if title != "dune" || year != 2021 {
		t.Fatalf("CleanQueryWithRecognition = %q/%d, want dune/2021", title, year)
	}
}

func TestValidateRecognitionWordURLRejectsUnsafeTargets(t *testing.T) {
	tests := []string{
		"file:///etc/passwd",
		"https://user:pass@example.com/words.txt",
		"http://localhost/words.txt",
		"http://127.0.0.1/words.txt",
		"http://10.0.0.1/words.txt",
		"http://172.16.0.1/words.txt",
		"http://192.168.1.1/words.txt",
		"http://[::1]/words.txt",
	}
	for _, rawURL := range tests {
		if err := validateRecognitionWordURL(t.Context(), rawURL); err == nil {
			t.Fatalf("validateRecognitionWordURL(%q) succeeded, want rejection", rawURL)
		}
	}
}

func TestValidateRecognitionWordURLAllowsPublicHTTPTargets(t *testing.T) {
	if err := validateRecognitionWordURL(t.Context(), "https://1.1.1.1/words.txt"); err != nil {
		t.Fatalf("public IP should be allowed: %v", err)
	}
	if err := validateRecognitionWordURL(t.Context(), "http://8.8.8.8/words.txt"); err != nil {
		t.Fatalf("public HTTP IP should be allowed: %v", err)
	}
}

func TestRecognitionWordDialRejectsUnsafeTargets(t *testing.T) {
	_, err := dialRecognitionWordContext(t.Context(), "tcp", "127.0.0.1:80")
	if err == nil || !strings.Contains(err.Error(), "restricted address") {
		t.Fatalf("dialRecognitionWordContext err = %v, want restricted address", err)
	}
}
