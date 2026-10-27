// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package execvemap

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"
)

// Operations of the execve_map_access program, see bpf_execve_map_access.c.
const (
	accessLookup = 0
	accessUpdate = 1
)

// accessCtx is struct execve_map_access, the value comes first.
type accessCtx struct {
	Value ExecveValue
	Op    uint32
	Pid   uint32
}

// accessProgram returns the execve_map_access program, see
// RegisterAccessProgramAtInit.
var accessProgram = func() *ebpf.Program { return nil }

// RegisterAccessProgramAtInit registers fn to return the loaded
// execve_map_access program, used to access a task storage execve_map.
//
// It must only be called at init time.
func RegisterAccessProgramAtInit(fn func() *ebpf.Program) {
	accessProgram = fn
}

func access(op, pid uint32, v *ExecveValue) error {
	prog := accessProgram()
	if prog == nil {
		return errors.New("execve_map access program is not loaded")
	}

	ctx := accessCtx{Value: *v, Op: op, Pid: pid}
	ret, err := prog.Run(&ebpf.RunOptions{Context: &ctx, ContextOut: &ctx})
	if err != nil {
		return fmt.Errorf("running execve_map access program: %w", err)
	}
	switch errno := unix.Errno(-int32(ret)); errno {
	case 0:
	case unix.ENOENT, unix.ESRCH:
		return fmt.Errorf("pid %d: %w", pid, ebpf.ErrKeyNotExist)
	default:
		return fmt.Errorf("pid %d: %w", pid, errno)
	}

	if op == accessLookup {
		*v = ctx.Value
	}
	return nil
}

// Lookup looks up the entry of pid in m, the execve_map.
//
// A task storage execve_map is keyed by pidfd rather than by pid, and pidfds
// are bound to the pid namespace of tetragon, which usually is not the host
// one. Its entries are accessed by the execve_map_access program instead.
func Lookup(m *ebpf.Map, pid uint32, v *ExecveValue) error {
	if m.Type() != ebpf.TaskStorage {
		return m.Lookup(&ExecveKey{Pid: pid}, v)
	}
	return access(accessLookup, pid, v)
}

// Put sets the entry of pid in m, the execve_map, see Lookup.
func Put(m *ebpf.Map, pid uint32, v *ExecveValue) error {
	if m.Type() != ebpf.TaskStorage {
		return m.Put(&ExecveKey{Pid: pid}, v)
	}
	return access(accessUpdate, pid, v)
}
