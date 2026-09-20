package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch, goarm string
		want                string
	}{
		{"linux", "amd64", "", "sshm_Linux_x86_64.tar.gz"},
		{"linux", "arm64", "", "sshm_Linux_arm64.tar.gz"},
		{"linux", "386", "", "sshm_Linux_i386.tar.gz"},
		{"linux", "arm", "", "sshm_Linux_armv7.tar.gz"},
		{"linux", "arm", "6", "sshm_Linux_armv6.tar.gz"},
		{"darwin", "amd64", "", "sshm_Darwin_x86_64.tar.gz"},
		{"darwin", "arm64", "", "sshm_Darwin_arm64.tar.gz"},
		{"windows", "amd64", "", "sshm_Windows_x86_64.zip"},
		{"windows", "386", "", "sshm_Windows_i386.zip"},
	}
	for _, c := range cases {
		got, err := assetName(c.goos, c.goarch, c.goarm)
		if err != nil {
			t.Fatalf("assetName(%s,%s,%s): %v", c.goos, c.goarch, c.goarm, err)
		}
		if got != c.want {
			t.Errorf("assetName(%s,%s,%s) = %q, want %q", c.goos, c.goarch, c.goarm, got, c.want)
		}
	}

	if _, err := assetName("plan9", "amd64", ""); err == nil {
		t.Error("expected error for unsupported OS")
	}
	if _, err := assetName("linux", "mips", ""); err == nil {
		t.Error("expected error for unsupported architecture")
	}
}

func TestExtractTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	write := func(name string, content string) {
		t.Helper()
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	write("LICENSE", "license text")
	write("sshm", "binary-content")

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractBinary("sshm_Linux_x86_64.tar.gz", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary-content" {
		t.Fatalf("extracted %q, want binary-content", got)
	}
}

func TestExtractZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("sshm.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("windows-binary")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractBinary("sshm_Windows_x86_64.zip", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "windows-binary" {
		t.Fatalf("extracted %q, want windows-binary", got)
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sshm")
	if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(path, []byte("new-binary")); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("content = %q, want new-binary", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
}
