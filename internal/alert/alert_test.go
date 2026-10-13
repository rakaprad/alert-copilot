package alert

import "testing"

func TestFingerprintStableAndLabelOrderIndependent(t *testing.T) {
	a := Alert{Source: "generic", Name: "DiskFull", Target: "db-1", Labels: map[string]string{"a": "1", "b": "2"}}
	b := Alert{Source: "generic", Name: "DiskFull", Target: "db-1", Labels: map[string]string{"b": "2", "a": "1"}}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("fingerprint should not depend on map order")
	}
	b.Target = "db-2"
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("different targets must produce different fingerprints")
	}
}

func TestFingerprintPrefersSourceID(t *testing.T) {
	a := Alert{Source: "alertmanager", Name: "X", ID: "abc123"}
	if got := a.Fingerprint(); got != "alertmanager:abc123" {
		t.Fatalf("got %q", got)
	}
}
