//go:build !windows

package videorepair

import (
	"fmt"
	"syscall"
)

func freeDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("파일 시스템 정보를 읽을 수 없습니다: %w", err)
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
