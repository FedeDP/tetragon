// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package base

import (
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/cilium/tetragon/pkg/bpf"
	"github.com/cilium/tetragon/pkg/config"
	"github.com/cilium/tetragon/pkg/logger"
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

func init() {
	program.OverrideMapSpecAtInit(ExecveMap.Name, func(spec *ebpf.MapSpec) {
		if !ExecveMapTaskStorage() {
			return
		}
		spec.Type = ebpf.TaskStorage
		spec.MaxEntries = 0
		spec.Flags |= unix.BPF_F_NO_PREALLOC
	})
}

// setupExecveMapTaskStorage selects the programs needed by a task storage
// execve_map, and returns false if the execve_map is a hash map.
func setupExecveMapTaskStorage() bool {
	if !ExecveMapTaskStorage() {
		return false
	}

	// The wake_up_new_task kprobe has no BTF pointer to the new task,
	// which the task storage helpers need.
	Fork.Name = "bpf_fork_v61.o"
	Fork.Attach = "sched_process_fork"
	Fork.Label = "tp_btf/sched_process_fork"
	Fork.Type = "tp_btf"

	// Socket programs can't get to the tasks.
	ExecveMapUpdate.Name = "bpf_execve_map_update_v61.o"
	ExecveMapUpdate.Attach = "syscall"
	ExecveMapUpdate.Label = "syscall"
	return true
}

func GetDefaultPrograms() []*program.Program {
	progs := []*program.Program{
		Exit,
		Fork,
		Execve,
		ExecveBprmCommit,
		ExecveMapUpdate,
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
