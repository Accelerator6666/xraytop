package system

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFindXrayProcesses(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		pid  int
		name string
	}{{123, "xray"}, {42, "nginx"}, {9, "xray"}} {
		dir := filepath.Join(root, strconv.Itoa(tc.pid))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(tc.name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "statm"), []byte("100 25 0 0 0 0 0\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FindXrayProcesses(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PID != 9 || got[1].PID != 123 {
		t.Fatalf("unexpected processes: %+v", got)
	}
	if got[0].RSSBytes != uint64(25*os.Getpagesize()) {
		t.Fatalf("unexpected rss: %d", got[0].RSSBytes)
	}
}

func TestFormatBytes(t *testing.T) {
	if s := FormatBytes(1536); s != "1.5 KiB" {
		t.Fatalf("unexpected size: %q", s)
	}
}
