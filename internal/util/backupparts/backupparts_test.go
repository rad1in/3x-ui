package backupparts

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"testing"
)

func gunzip(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSmallBackupGoesAsIs(t *testing.T) {
	data := []byte("SQLite format 3\x00 tiny")
	parts, err := Split("x.db", data, 1024)
	if err != nil || len(parts) != 1 || parts[0].Name != "x.db" || !bytes.Equal(parts[0].Data, data) {
		t.Fatalf("parts = %+v, err = %v; want the file untouched", parts, err)
	}
}

func TestOversizedBackupIsCompressed(t *testing.T) {
	data := bytes.Repeat([]byte("client row "), 10_000)
	parts, err := Split("x.db", data, 4096)
	if err != nil || len(parts) != 1 || parts[0].Name != "x.db.gz" || len(parts[0].Data) > 4096 {
		t.Fatalf("parts = %d (%v), err = %v; want one x.db.gz under the cap", len(parts), names(parts), err)
	}
	if !bytes.Equal(gunzip(t, parts[0].Data), data) {
		t.Fatal("x.db.gz does not unpack to the backup")
	}
}

// Rejoining with `cat x.db.gz.part* | gunzip` must give the backup back, so
// the parts sort in order and each fits the cap.
func TestIncompressibleBackupIsCutIntoOrderedParts(t *testing.T) {
	data := make([]byte, 25_000)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	parts, err := Split("x.db", data, 2048)
	if err != nil || len(parts) < 12 {
		t.Fatalf("parts = %d, err = %v", len(parts), err)
	}
	var joined []byte
	for i, p := range parts {
		if len(p.Data) > 2048 {
			t.Fatalf("%s is %d bytes, over the cap", p.Name, len(p.Data))
		}
		if i > 0 && p.Name <= parts[i-1].Name {
			t.Fatalf("%s sorts before %s; cat part* would scramble them", p.Name, parts[i-1].Name)
		}
		joined = append(joined, p.Data...)
	}
	if parts[0].Name != "x.db.gz.part01" {
		t.Fatalf("first part named %q", parts[0].Name)
	}
	if !bytes.Equal(gunzip(t, joined), data) {
		t.Fatal("rejoined parts do not unpack to the backup")
	}
}

func names(parts []Part) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p.Name
	}
	return out
}
