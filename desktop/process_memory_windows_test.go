//go:build windows

package main

import (
	"os"
	"testing"
)

func TestMeasureProcessTreePrivateMemoryIncludesRootProcess(t *testing.T) {
	measured, err := measureProcessTreePrivateMemory(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if measured.ProcessTreePrivateBytes <= 0 {
		t.Fatalf("process tree private bytes = %d", measured.ProcessTreePrivateBytes)
	}
}
