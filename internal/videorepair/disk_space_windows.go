//go:build windows

package videorepair

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	kernel32Disk                 = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExWProcedure = kernel32Disk.NewProc("GetDiskFreeSpaceExW")
)

func freeDiskSpace(path string) (uint64, error) {
	pathPointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("디스크 경로를 변환할 수 없습니다: %w", err)
	}

	var available uint64
	result, _, callErr := getDiskFreeSpaceExWProcedure.Call(
		uintptr(unsafe.Pointer(pathPointer)),
		uintptr(unsafe.Pointer(&available)),
		0,
		0,
	)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return 0, fmt.Errorf("디스크 여유 공간을 확인할 수 없습니다: %w", callErr)
		}
		return 0, fmt.Errorf("디스크 여유 공간을 확인할 수 없습니다")
	}
	return available, nil
}
