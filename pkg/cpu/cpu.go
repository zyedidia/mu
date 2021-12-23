package cpu

import (
	"runtime"

	"github.com/klauspost/cpuid/v2"
)

// NumCores returns the number of physical cores on this machine (hyperthreads
// not included).
func NumCores() int {
	if cpuid.CPU.PhysicalCores == 0 {
		return runtime.NumCPU()
	}
	return cpuid.CPU.PhysicalCores
}

// NumThreads returns the number of logical cores on this machine (hyperthreads
// included).
func NumThreads() int {
	return runtime.NumCPU()
}
