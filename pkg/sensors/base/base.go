// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package base

import (
	"fmt"
	"log"
	"sync"
	"testing"
	"unsafe"

	"github.com/cilium/tetragon/pkg/bpf"
	"github.com/cilium/tetragon/pkg/config"
	"github.com/cilium/tetragon/pkg/errmetrics"
	"github.com/cilium/tetragon/pkg/execvemapupdater"
	"github.com/cilium/tetragon/pkg/ksyms"
	"github.com/cilium/tetragon/pkg/logger"
	"github.com/cilium/tetragon/pkg/mbset"
	"github.com/cilium/tetragon/pkg/option"
	"github.com/cilium/tetragon/pkg/sensors"
	"github.com/cilium/tetragon/pkg/sensors/exec/execvemap"
	"github.com/cilium/tetragon/pkg/sensors/program"
	"github.com/cilium/tetragon/pkg/strutils"
)

const (
	execveMapMaxEntries = 32768
	RingBufMapName      = "tg_rb_events"
)

var (
	Execve = program.Builder(
		config.ExecObj(),
		"sched/sched_process_exec",
		"raw_tracepoint/sys_execve",
		"event_execve",
		"execve",
	).SetPolicy(sensors.BaseSensorName)

	ExecveMapUpdate = program.Builder(
		config.ExecUpdateObj(),
		"socket",
		"socket",
		"execve_map_update",
		"socket",
	).SetPolicy(sensors.BaseSensorName)

	// BpfProcScan is run once at startup to populate execve_map with the
	// processes already running, see RunProcScan. It's only loaded when
	// BPFProcScanEnabled() is true.
	BpfProcScan = program.Builder(
		"bpf_proc_scan_v61.o",
		"syscall",
		"syscall",
		"proc_scan",
		"socket",
	).SetPolicy(sensors.BaseSensorName)

	// ExecveBeginNewExec keeps the task storage execve_map entry of a
	// process across de_thread(). It's only loaded when
	// ExecveMapTaskStorage() is true.
	ExecveBeginNewExec = program.Builder(
		"bpf_execve_begin_v61.o",
		"begin_new_exec",
		"kprobe/begin_new_exec",
		"tg_kp_begin_new_exec",
		"kprobe",
	).SetPolicy(sensors.BaseSensorName)

	// ExecveMapAccess is used by userspace to access the execve_map
	// entries when it's a task storage map, see execvemap.Lookup.
	ExecveMapAccess = program.Builder(
		"bpf_execve_map_access_v61.o",
		"syscall",
		"syscall",
		"execve_map_access",
		"socket",
	).SetPolicy(sensors.BaseSensorName)

	ExecveBprmCommit = program.Builder(
		config.ExecveBprmCommitObj(),
		"security_bprm_committing_creds",
		"kprobe/security_bprm_committing_creds",
		"tg_kp_bprm_committing_creds",
		"kprobe",
	).SetPolicy(sensors.BaseSensorName)

	Exit = program.Builder(
		config.ExitObj(),
		"acct_process",
		"kprobe/acct_process",
		"event_exit",
		"kprobe",
	).SetPolicy(sensors.BaseSensorName)

	Fork = program.Builder(
		config.ForkObj(),
		"wake_up_new_task",
		"kprobe/wake_up_new_task",
		"kprobe_pid_clear",
		"kprobe",
	).SetPolicy(sensors.BaseSensorName)

	// SchedProcessFork replaces Fork when ExecveMapTaskStorage() is true:
	// the task storage helpers need a BTF pointer to the new task, which
	// the wake_up_new_task kprobe doesn't have.
	SchedProcessFork = program.Builder(
		"bpf_fork_v61.o",
		"sched_process_fork",
		"tp_btf/sched_process_fork",
		"event_sched_process_fork",
		"tp_btf",
	).SetPolicy(sensors.BaseSensorName)

	// ExecveMapUpdateTask replaces ExecveMapUpdate when
	// ExecveMapTaskStorage() is true: socket programs can't get to the
	// tasks.
	ExecveMapUpdateTask = program.Builder(
		"bpf_execve_map_update_v61.o",
		"syscall",
		"syscall",
		"execve_map_update_task",
		"socket",
	).SetPolicy(sensors.BaseSensorName)

	/* Event Ring map */
	TCPMonMap     = program.MapBuilder("tcpmon_map", Execve)
	RingBufEvents = program.MapBuilder(RingBufMapName, Execve, Exit, Fork, SchedProcessFork)
	/* Networking and Process Monitoring maps */
	// The execve_map is either a hash map keyed by pid, or a task storage
	// map when ExecveMapTaskStorage() is true. The BPF objects declare both,
	// userspace only loads the one in use: ExecveMap, set by setupSensor().
	execveHashMap        = program.MapBuilder(execvemap.HashMapName, Execve, Exit, Fork, ExecveBprmCommit, ExecveMapUpdate)
	execveTaskStorageMap = program.MapBuilder(execvemap.TaskStorageMapName, Execve, Exit, SchedProcessFork, ExecveBprmCommit, ExecveMapUpdateTask, BpfProcScan, ExecveBeginNewExec, ExecveMapAccess)
	ExecveMap            = execveHashMap
	ExecveKernelMap      = program.MapBuilder("execve_map_kernel", Execve, ExecveMapAccess)
	ExecveTailCallsMap   = program.MapBuilderProgram("execve_calls", Execve)
	ExecveMapUpdateData  = program.MapBuilder("execve_map_update_data", ExecveMapUpdate, ExecveMapUpdateTask)
	BpfProcScanMap       = program.MapBuilder("tg_proc_scan_map", BpfProcScan)

	ExecveJoinMap = program.MapBuilder("tg_execve_joined_info_map", ExecveBprmCommit)

	ParentBinariesMap = program.MapBuilder("tg_parents_bin", Execve, Exit)

	/* Tetragon runtime configuration */
	TetragonConfMap = program.MapBuilder("tg_conf_map", Execve)

	/* Internal statistics for debugging */
	ExecveStats        = program.MapBuilder("execve_map_stats", Execve)
	ExecveJoinMapStats = program.MapBuilder("tg_execve_joined_info_map_stats", ExecveBprmCommit)
	StatsMap           = program.MapBuilder("tg_stats_map", Execve)

	MatchBinariesSetMap = program.MapBuilder(mbset.MapName, Execve)
	MatchBinariesGenMap = program.MapBuilder(mbset.GenName, Execve)

	ErrMetricsMap = program.MapBuilder(errmetrics.MapName, Execve)
)

func parseExecveMapSize(str string) (int, error) {
	// set entries based on size
	size, err := strutils.ParseSize(str)
	if err != nil {
		return 0, err
	}
	val := size / int(unsafe.Sizeof(execvemap.ExecveValue{}))
	return val, nil
}

func GetExecveEntries(configEntries int, configSize string) int {
	// Setup execve_map max entries
	if configEntries != 0 && len(configSize) != 0 {
		log.Fatal("Both ExecveMapEntries and ExecveMapSize set, confused..")
	}

	var (
		entries int
		err     error
	)

	if configEntries != 0 {
		entries = configEntries
	} else if len(configSize) != 0 {
		if entries, err = parseExecveMapSize(configSize); err != nil {
			log.Fatal("Failed to parse ExecveMapSize value")
		}
	} else {
		entries = execveMapMaxEntries
	}

	return entries
}

func setupSensor() {
	// exit program function
	ks, err := ksyms.KernelSymbols()
	if err == nil {
		hasAcctProcess := ks.IsAvailable("acct_process")
		hasDisassociateCtty := ks.IsAvailable("disassociate_ctty")

		/* Preffer acct_process over disassociate_ctty */
		if hasAcctProcess {
			Exit.Attach = "acct_process"
			Exit.Label = "kprobe/acct_process"
		} else if hasDisassociateCtty {
			Exit.Attach = "disassociate_ctty"
			Exit.Label = "kprobe/disassociate_ctty"
		} else {
			log.Fatal("Failed to detect exit probe symbol.")
		}
	}
	logger.GetLogger().Info("Exit probe on " + Exit.Attach)

	entries := GetExecveEntries(option.Config.ExecveMapEntries, option.Config.ExecveMapSize)

	// The BPF proc scan is only used along with the task storage execve_map,
	// see ExecveMapTaskStorage().
	if ExecveMapTaskStorage() {
		ExecveMap = execveTaskStorageMap
		// The scan records every running process, so don't go below the
		// default execve_map size. The map is not preallocated, so it only
		// uses memory for the running processes.
		BpfProcScanMap.SetMaxEntries(max(entries, execveMapMaxEntries))
		logger.GetLogger().Info("execve_map uses task storage, its configured entries only size the startup scan")
	} else {
		ExecveMap = execveHashMap
		ExecveMap.SetMaxEntries(entries)
		if option.Config.ExecveMapNoPrealloc {
			ExecveMap.SetFlag(bpf.BPF_F_NO_PREALLOC)
		}
		logger.GetLogger().Info(fmt.Sprintf("Set execve_map entries %d", entries),
			"size", strutils.SizeWithSuffix(entries*int(unsafe.Sizeof(execvemap.ExecveValue{}))))
	}
	unpinInactiveExecveMap()

	if !config.EnableV511Progs() && option.Config.EnableProcessEnvironmentVariables {
		Execve.RewriteConstants["ENV_VARS_ENABLED"] = uint8(1)
	}

	if !config.EnableV511Progs() && option.Config.ParentsMapEnabled {
		Execve.RewriteConstants["PARENTS_MAP_ENABLED"] = uint8(1)
	}

	if !config.EnableV511Progs() && len(option.Config.BPFDebugAreas.Values) > 0 {
		Execve.RewriteConstants["BPF_DEBUG_ENABLED"] = option.Config.BPFDebugAreas.ToBPFConfig()
	}

	// v5.11 and v6.1 bpf objects use task storage for the parents map,
	// which has no max entries.
	if option.Config.ParentsMapEnabled && !config.EnableV61Progs() && !config.EnableV511Progs() {
		entries = GetExecveEntries(option.Config.ParentsMapEntries, option.Config.ParentsMapSize)
		ParentBinariesMap.SetMaxEntries(entries)
		logger.GetLogger().Info(fmt.Sprintf("Set parents_map entries %d", entries),
			"size", strutils.SizeWithSuffix(entries*int(unsafe.Sizeof(execvemap.ExecveValue{}))))
	}
}

func GetExecveMap() *program.Map {
	return ExecveMap
}

func GetExecveMapStats() *program.Map {
	return ExecveStats
}

func GetTetragonConfMap() *program.Map {
	return TetragonConfMap
}

func initBaseSensor() *sensors.Sensor {
	sensor := sensors.Sensor{
		Name: sensors.BaseSensorName,
	}
	setupSensor()
	if config.EnableLargeProgs() {
		mbset.SetMBSetUpdater(&execvemapupdater.ExecveMapUpdater{
			Load:      execveMapUpdateProgram(),
			Map:       ExecveMapUpdateData,
			ExecveMap: ExecveMap,
		})
	}
	sensor.Progs = GetDefaultPrograms()
	sensor.Maps = GetDefaultMaps()
	return ApplyExtensions(&sensor)
}

func initBaseSensorFn() func(tb testing.TB) *sensors.Sensor {
	var (
		s *sensors.Sensor
		m sync.Mutex
	)
	return func(tb testing.TB) *sensors.Sensor {
		m.Lock()
		defer m.Unlock()
		if s == nil {
			s = initBaseSensor()
			tb.Cleanup(func() {
				tb.Logf("cleanup: unloading base sensor")
				s.Unload(true)
				s = nil
			})
		}
		return s
	}
}

var (
	// GetInitialSensor returns the base sensor
	GetInitialSensor     = sync.OnceValue(initBaseSensor)
	GetInitialSensorTest = initBaseSensorFn()
)
