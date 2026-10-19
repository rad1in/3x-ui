package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

func TestSubFetchLog(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	var log SubFetchService
	at := time.UnixMilli(1_700_000_000_000)

	if err := log.Record("s1", "198.51.100.7", "v2rayNG/1.10.2", "raw", at); err != nil {
		t.Fatal(err)
	}
	if err := log.Record("s1", "198.51.100.7", "v2rayNG/1.10.2", "raw", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := log.Record("s1", "203.0.113.9", "Hiddify/2.5", "json", at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := log.Record("other", "198.51.100.7", "v2rayNG/1.10.2", "raw", at); err != nil {
		t.Fatal(err)
	}

	got, err := log.List("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (the same IP+app+format collapses): %+v", len(got), got)
	}
	if got[0].IP != "203.0.113.9" || got[0].Format != "json" || got[0].Count != 1 {
		t.Fatalf("newest first: got %+v", got[0])
	}
	if e := got[1]; e.Count != 2 || e.FirstSeen != at.UnixMilli() || e.LastSeen != at.Add(time.Hour).UnixMilli() || e.UserAgent != "v2rayNG/1.10.2" {
		t.Fatalf("repeat fetch must bump count and last seen only: %+v", e)
	}
}

func TestSubFetchLogKeepsOnlyTheNewestDistinctFetchers(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	var log SubFetchService
	at := time.UnixMilli(1_700_000_000_000)
	for i := range subFetchKeep + 5 {
		ip := "10.0.0." + string(rune('A'+i%26)) + string(rune('a'+i/26))
		if err := log.Record("s1", ip, "ua", "raw", at.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := log.List("s1")
	if len(got) != subFetchKeep {
		t.Fatalf("kept %d entries, want %d", len(got), subFetchKeep)
	}
	if want := at.Add(time.Duration(subFetchKeep+4) * time.Minute).UnixMilli(); got[0].LastSeen != want {
		t.Fatalf("newest entry lastSeen = %d, want %d", got[0].LastSeen, want)
	}
}

func TestSubFetchLogTruncatesUserAgent(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	var log SubFetchService
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'a'
	}
	if err := log.Record("s1", "198.51.100.7", string(long), "raw", time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ := log.List("s1")
	if len(got[0].UserAgent) != subFetchMaxUserAgent {
		t.Fatalf("user agent length = %d, want %d", len(got[0].UserAgent), subFetchMaxUserAgent)
	}
}
