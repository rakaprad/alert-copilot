package source

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

func open(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestParseAlertmanager(t *testing.T) {
	alerts, err := ParseAlertmanager(open(t, "alertmanager.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 {
		t.Fatalf("want 2 alerts, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Name != "HighCPUUsage" || a.Status != alert.StatusFiring || a.Severity != "warning" {
		t.Errorf("unexpected alert: %+v", a)
	}
	if a.Target != "10.0.3.17:8080" || a.Value != "94%" || a.ID != "c6b1f0e5a1d2e3f4" {
		t.Errorf("unexpected target/value/id: %+v", a)
	}
	if a.Labels["pod"] != "payments-api-7d9c6b5f8-x2kqj" {
		t.Errorf("pod label missing: %v", a.Labels)
	}
	if _, ok := a.Labels["alertname"]; ok {
		t.Error("alertname should be promoted out of labels")
	}
	if alerts[1].Status != alert.StatusResolved {
		t.Errorf("second alert should be resolved")
	}
}

func TestParseAlertmanagerRejectsGarbage(t *testing.T) {
	for _, body := range []string{`not json`, `{"alerts":[]}`} {
		if _, err := ParseAlertmanager(strings.NewReader(body)); err == nil {
			t.Errorf("expected error for %q", body)
		}
	}
}

func TestParseAliCloud(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/alicloud.form")
	if err != nil {
		t.Fatal(err)
	}
	form, err := url.ParseQuery(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	alerts, err := ParseAliCloud(form)
	if err != nil {
		t.Fatal(err)
	}
	a := alerts[0]
	if a.Name != "ECS CPU Utilization High" || a.Status != alert.StatusFiring {
		t.Errorf("unexpected: %+v", a)
	}
	if a.Severity != "critical" || a.Target != "prod-web-01" || a.Value != "92.5" {
		t.Errorf("unexpected severity/target/value: %+v", a)
	}
	if a.Labels["regionName"] != "Indonesia (Jakarta)" {
		t.Errorf("region label: %v", a.Labels)
	}
	if a.StartsAt.IsZero() {
		t.Error("timestamp not parsed")
	}

	// A repeat callback with a new value and duration is the same alert.
	form.Set("curValue", "97.1")
	form.Set("lastTime", "15 minutes")
	again, _ := ParseAliCloud(form)
	if again[0].Fingerprint() != a.Fingerprint() {
		t.Error("repeat callbacks should share a fingerprint")
	}
}

func TestParseAliCloudResolvedAndMissingName(t *testing.T) {
	raw, _ := os.ReadFile("../../testdata/alicloud-ok.form")
	form, _ := url.ParseQuery(strings.TrimSpace(string(raw)))
	alerts, err := ParseAliCloud(form)
	if err != nil {
		t.Fatal(err)
	}
	if alerts[0].Status != alert.StatusResolved {
		t.Error("alertState=OK should map to resolved")
	}
	if _, err := ParseAliCloud(url.Values{}); err == nil {
		t.Error("expected error for empty form")
	}
}

func TestParseGeneric(t *testing.T) {
	alerts, err := ParseGeneric(open(t, "generic.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := alerts[0]
	if a.Source != "generic" || a.Name != "BackupJobFailed" || a.Status != alert.StatusFiring || a.Target != "pg-primary-01" {
		t.Errorf("unexpected: %+v", a)
	}

	list, err := ParseGeneric(strings.NewReader(`[{"name":"A"},{"name":"B","status":"resolved","source":"spoofed"}]`))
	if err != nil || len(list) != 2 {
		t.Fatalf("array form: %v %v", list, err)
	}
	if list[1].Status != alert.StatusResolved || list[1].Source != "generic" {
		t.Errorf("unexpected: %+v", list[1])
	}
	if _, err := ParseGeneric(strings.NewReader(`{"summary":"no name"}`)); err == nil {
		t.Error("expected error when name missing")
	}
}
