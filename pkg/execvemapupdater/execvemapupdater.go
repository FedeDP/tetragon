// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package execvemapupdater

import (
	"errors"

	"github.com/cilium/ebpf"

	"github.com/cilium/tetragon/pkg/config"
	"github.com/cilium/tetragon/pkg/kernels"
	"github.com/cilium/tetragon/pkg/sensors/program"
)

const (
	maxPids = 32768
)

type ExecveMapUpdater struct {
	Load *program.Program
	Map  *program.Map
	// ExecveMap is the execve_map in use
	ExecveMap *program.Map
}

func (upd *ExecveMapUpdater) ExecveMapName() string {
	return upd.ExecveMap.Name
}

type UpdateData struct {
	bit  uint32
	cnt  uint32
	pids [maxPids]uint32
}

func getRoundCnt() uint32 {
	if config.EnableV61Progs() {
		return maxPids
	} else if kernels.MinKernelVersion("5.11") {
		return 1024
	} else if config.EnableLargeProgs() {
		return 1024
	}
	return 0
}

func (upd *ExecveMapUpdater) MBSetBitClear(bit uint32, pids []uint32) error {
	prog := upd.Load.Prog

	var opts ebpf.RunOptions
	// syscall programs (used with a task storage execve_map) take no data
	if prog.Type() != ebpf.Syscall {
		// not used, but needed to pass program test run
		opts.Data = make([]byte, 20)
	}

	key := uint32(0)
	val := UpdateData{
		bit: bit,
	}

	run := func() error {
		if err := upd.Map.MapHandle.Update(key, val, 0); err != nil {
			return errors.New("failed to update data map value")
		}

		ret, err := prog.Run(&opts)

		// executed, but failed in bpf program above
		if err != nil || ret != 0 {
			return errors.New("failed to update map value")
		}
		return nil
	}

	var err error

	maxCnt := getRoundCnt()

	for _, pid := range pids {
		if val.cnt < maxCnt {
			val.pids[val.cnt] = pid
			val.cnt++
			continue
		}

		// update every maxCnt keys
		if err := run(); err != nil {
			return err
		}
		val.cnt = 0
	}

	// ... and the rest
	if val.cnt > 0 {
		err = run()
	}

	return err
}

// MBSetBitClearAll clears bit from all the execve_map entries, and returns the
// number of entries that had it set. It needs the execve_map_update_task
// program, used with a task storage execve_map, which walks all the
// processes rather than a list of pids.
func (upd *ExecveMapUpdater) MBSetBitClearAll(bit uint32) (uint32, error) {
	prog := upd.Load.Prog
	if prog.Type() != ebpf.Syscall {
		return 0, errors.New("clearing the bit from all the entries needs the execve_map_update_task program")
	}

	if err := upd.Map.MapHandle.Update(uint32(0), UpdateData{bit: bit}, 0); err != nil {
		return 0, errors.New("failed to update data map value")
	}
	ret, err := prog.Run(&ebpf.RunOptions{})
	if err != nil || int32(ret) < 0 {
		return 0, errors.New("failed to update map values")
	}
	return ret, nil
}
