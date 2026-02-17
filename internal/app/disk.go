package app

import "syscall"

// diskUsage returns disk usage stats for the given path, or nil on error.
func diskUsage(path string) map[string]any {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	return map[string]any{
		"total_bytes":     total,
		"used_bytes":      used,
		"available_bytes": free,
	}
}
