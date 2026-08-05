//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type desktopProcessMemory struct {
	ProcessTreePrivateBytes int64
	WebViewPrivateBytes     int64
}

type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

var getProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

func measureDesktopProcessTreeMemory() (desktopProcessMemory, error) {
	return measureProcessTreePrivateMemory(uint32(os.Getpid()))
}

func measureProcessTreePrivateMemory(rootPID uint32) (desktopProcessMemory, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return desktopProcessMemory{}, fmt.Errorf("snapshot processes: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	type processEntry struct {
		pid    uint32
		parent uint32
		name   string
	}
	entries := make([]processEntry, 0, 64)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return desktopProcessMemory{}, fmt.Errorf("read process snapshot: %w", err)
	}
	for {
		entries = append(entries, processEntry{
			pid: entry.ProcessID, parent: entry.ParentProcessID,
			name: windows.UTF16ToString(entry.ExeFile[:]),
		})
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return desktopProcessMemory{}, fmt.Errorf("advance process snapshot: %w", err)
		}
	}

	owned := map[uint32]bool{rootPID: true}
	for changed := true; changed; {
		changed = false
		for _, candidate := range entries {
			if !owned[candidate.pid] && owned[candidate.parent] {
				owned[candidate.pid] = true
				changed = true
			}
		}
	}

	var result desktopProcessMemory
	for _, candidate := range entries {
		if !owned[candidate.pid] {
			continue
		}
		privateBytes, err := processPrivateBytes(candidate.pid)
		if err != nil {
			// WebView child processes can exit between the snapshot and the
			// memory read. A later sustained sample will observe replacements.
			continue
		}
		result.ProcessTreePrivateBytes += privateBytes
		if strings.EqualFold(candidate.name, "msedgewebview2.exe") {
			result.WebViewPrivateBytes += privateBytes
		}
	}
	return result, nil
}

func processPrivateBytes(pid uint32) (int64, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(handle)
	counters := processMemoryCountersEx{CB: uint32(unsafe.Sizeof(processMemoryCountersEx{}))}
	result, _, callErr := getProcessMemoryInfo.Call(
		uintptr(handle), uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB),
	)
	if result == 0 {
		return 0, callErr
	}
	return int64(counters.PrivateUsage), nil
}
