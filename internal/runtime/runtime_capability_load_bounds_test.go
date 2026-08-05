package runtime

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeCapabilityLoadRecordsAreCountAndByteBounded(t *testing.T) {
	t.Parallel()
	service := newRuntimeService()
	large := strings.Repeat("x", runtimeCapabilityLoadRecordTextBytes*4)
	for index := 0; index < 1000; index++ {
		service.setCapabilityLoadRecord(fmt.Sprintf("capability-%04d", index), runtimeCapabilityLoadRecord{
			State:       capabilityStateFailed,
			Diagnostics: large,
			Error:       large,
			Reason:      large,
			UpdatedAt:   int64(index + 1),
		})
	}

	records := cloneCapabilityLoadRecords(service.capabilityLoads)
	if len(records) != runtimeCapabilityLoadRecordMaxEntries {
		t.Fatalf("capability load record count = %d, want %d", len(records), runtimeCapabilityLoadRecordMaxEntries)
	}
	if _, ok := records["capability-0743"]; ok {
		t.Fatal("oldest capability load record was not evicted")
	}
	latest, ok := records["capability-0999"]
	if !ok {
		t.Fatal("latest capability load record was evicted")
	}
	for field, value := range map[string]string{"diagnostics": latest.Diagnostics, "error": latest.Error, "reason": latest.Reason} {
		if len(value) > runtimeCapabilityLoadRecordTextBytes {
			t.Fatalf("%s retained %d bytes, want <= %d", field, len(value), runtimeCapabilityLoadRecordTextBytes)
		}
	}
}
