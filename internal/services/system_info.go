package services

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SystemInfo contains system information
type SystemInfo struct {
	TotalMemory     uint64
	AvailableMemory uint64
	OS              string
	Arch            string
}

// GetSystemInfo retrieves system information
func GetSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Get memory info based on OS
	switch runtime.GOOS {
	case "linux":
		info.TotalMemory, info.AvailableMemory = getLinuxMemory()
	case "darwin":
		info.TotalMemory, info.AvailableMemory = getMacMemory()
	case "windows":
		info.TotalMemory, info.AvailableMemory = getWindowsMemory()
	default:
		info.TotalMemory = 16 * 1024 * 1024 * 1024 // Default to 16GB
		info.AvailableMemory = 8 * 1024 * 1024 * 1024 // Default to 8GB
	}

	return info, nil
}

// getLinuxMemory gets memory info on Linux
func getLinuxMemory() (total, available uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 16 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				total = val * 1024 // Convert from KB to bytes
			}
		case "MemAvailable:":
			if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				available = val * 1024 // Convert from KB to bytes
			}
		}
	}

	// If MemAvailable is not found, estimate it
	if available == 0 && total > 0 {
		available = total / 2 // Rough estimate
	}

	return total, available
}

// getMacMemory gets memory info on macOS
func getMacMemory() (total, available uint64) {
	// For macOS, we'll use a simplified approach
	// In a real implementation, you'd use syscalls or execute system commands
	return 16 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024
}

// getWindowsMemory gets memory info on Windows
func getWindowsMemory() (total, available uint64) {
	// For Windows, we'll use a simplified approach
	// In a real implementation, you'd use Windows API calls
	return 16 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024
}

// FormatMemory formats memory size in human readable format
func FormatMemory(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// CanRunModel checks if system has enough memory for a model
func CanRunModel(modelMemoryGB float64, systemInfo *SystemInfo) (bool, string) {
	requiredBytes := uint64(modelMemoryGB * 1024 * 1024 * 1024)
	
	// Add some overhead for system operations (1GB)
	requiredWithOverhead := requiredBytes + (1024 * 1024 * 1024)
	
	if systemInfo.AvailableMemory < requiredWithOverhead {
		return false, fmt.Sprintf("Model requires %s but only %s available", 
			FormatMemory(requiredBytes), 
			FormatMemory(systemInfo.AvailableMemory))
	}
	
	return true, ""
}