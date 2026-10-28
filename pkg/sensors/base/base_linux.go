// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package base

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/cilium/tetragon/pkg/bpf"
	"github.com/cilium/tetragon/pkg/config"
	"github.com/cilium/tetragon/pkg/logger"
	"github.com/cilium/tetragon/pkg/logger/logfields"
	"github.com/cilium/tetragon/pkg/option"
	"github.com/cilium/tetragon/pkg/sensors/program"
	"github.com/cilium/tetragon/pkg/strutils"
)

// BPFProcScanEnabled returns true if the startup process scan is done by the
// proc_scan BPF program instead of reading procfs.
func BPFProcScanEnabled() bool {
	return config.EnableV61Progs() && bpf.HasTaskIterAllProcs()
}

// ExecveMapTaskStorage returns true if execve_map is a task storage map rather
// than a hash map keyed by pid. That's the case when the BPF process scan is
// available: the procfs scan could only add the processes to it with pidfds,
// which don't work for the host processes when tetragon runs without hostPID.
// The kernels that support the scan have all the other helpers and kfuncs the
// task storage execve_map needs.
func ExecveMapTaskStorage() bool {
	return BPFProcScanEnabled()
}

// unpinInactiveExecveMap removes the pin of the execve_map not in use, which a
// previous run in the other mode may have left. The BPF objects reference both
// maps, so the loader would otherwise keep using the stale map, which for the
// hash map means its preallocated memory.
func unpinInactiveExecveMap() {
	inactive := execveHashMap
	if ExecveMap == execveHashMap {
		inactive = execveTaskStorageMap
	}
	pin := filepath.Join(bpf.MapPrefixPath(), inactive.Name)
	if err := os.Remove(pin); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.GetLogger().Warn("Failed to remove the inactive execve_map pin", "pin", pin, logfields.Error, err)
	}
}

// forkProgram returns the fork program for the execve_map type.
func forkProgram() *program.Program {
	if ExecveMapTaskStorage() {
		return SchedProcessFork
	}
	return Fork
}

// execveMapUpdateProgram returns the program used by the mbset updater for
// the execve_map type.
func execveMapUpdateProgram() *program.Program {
	if ExecveMapTaskStorage() {
		return ExecveMapUpdateTask
	}
	return ExecveMapUpdate
}

func GetDefaultPrograms() []*program.Program {
	progs := []*program.Program{
		Exit,
		forkProgram(),
		Execve,
		ExecveBprmCommit,
		execveMapUpdateProgram(),
	}
	if BPFProcScanEnabled() {
		progs = append(progs, BpfProcScan)
	}
	if ExecveMapTaskStorage() {
		progs = append(progs, ExecveBeginNewExec, ExecveMapAccess)
	}
	return progs
}

func GetDefaultMaps() []*program.Map {
	maps := []*program.Map{
		ExecveMap,
		ExecveJoinMap,
		ExecveStats,
		ExecveJoinMapStats,
		ExecveMapUpdateData,
		TCPMonMap,
		TetragonConfMap,
		StatsMap,
		MatchBinariesSetMap,
		MatchBinariesGenMap,
		ErrMetricsMap,
	}

	if !config.EnableLargeProgs() {
		maps = append(maps, ExecveTailCallsMap)
	}

	if option.Config.ParentsMapEnabled {
		maps = append(maps, ParentBinariesMap)
	}
	if BPFProcScanEnabled() {
		maps = append(maps, BpfProcScanMap)
	}
	if ExecveMapTaskStorage() {
		maps = append(maps, ExecveKernelMap)
	}
	if config.EnableV511Progs() {
		maps = append(maps, RodataConfigMap)
	}

	// The BPF ring buffer is available from v5.8, but rather than add another set of
	// kernel-version-specific objects, let's set the gate at v5.11 as we already have
	// objects for that version number.
	if config.EnableV511Progs() && !option.Config.UsePerfRingBuffer {
		rbSize := config.GetRBSize()
		RingBufEvents.SetMaxEntries(rbSize)
		logger.GetLogger().Info("BPF ring buffer size (bytes)", "total", strutils.SizeWithSuffix(rbSize))
		maps = append(maps, RingBufEvents)
	}
	return maps

}
