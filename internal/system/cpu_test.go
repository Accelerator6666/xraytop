package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCPUSample(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "123"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stat"), []byte("cpu  100 20 30 400 5\ncpu0 1 1 1 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	fields := []byte("123 (xray worker) S 1 2 3 4 5 6 7 8 9 10 25 35 14 15 16 17 18 19 20\n")
	if err := os.WriteFile(filepath.Join(root, "123", "stat"), fields, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCPUSample(root, 123)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 555 || got.Process != 60 {
		t.Fatalf("sample=%+v", got)
	}
}
