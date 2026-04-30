package tools

import (
	"reflect"
	"testing"
	"time"
)

func TestExtensionLifecycleRecordNormalizesStateIdentityAndCapabilities(t *testing.T) {
	updated := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)
	record := NormalizeExtensionLifecycleRecord(ExtensionLifecycleRecord{
		Type:             " Tool ",
		Source:           " Dynamic ",
		Name:             " Search ",
		Version:          " 1.2.3 ",
		State:            " ACTIVE ",
		Capabilities:     []string{"invoke", "invoke", " reload ", ""},
		LastError:        "  ",
		LastUpdated:      updated,
		RecoveryBehavior: " RebuildFromDiscovery ",
	})

	if record.Key() != "tool|dynamic|Search" {
		t.Fatalf("key = %q, want normalized lifecycle identity", record.Key())
	}
	if record.State != ExtensionStateActive {
		t.Fatalf("state = %q, want active", record.State)
	}
	if !reflect.DeepEqual(record.Capabilities, []string{"invoke", "reload"}) {
		t.Fatalf("capabilities = %#v, want compact sorted capabilities", record.Capabilities)
	}
	if record.LastError != "" || record.RecoveryBehavior != "rebuildFromDiscovery" {
		t.Fatalf("record = %#v, want trimmed error and normalized recovery behavior", record)
	}
}

func TestExtensionLifecycleRecordDefaultsInvalidStateToDiscovered(t *testing.T) {
	record := NormalizeExtensionLifecycleRecord(ExtensionLifecycleRecord{
		Type:   "skill",
		Source: "skills",
		Name:   "review",
		State:  "unknown",
	})

	if record.State != ExtensionStateDiscovered {
		t.Fatalf("state = %q, want discovered", record.State)
	}
}
