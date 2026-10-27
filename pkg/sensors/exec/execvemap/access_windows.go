// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package execvemap

import (
	"github.com/cilium/ebpf"
)

// Lookup looks up the entry of pid in m, the execve_map.
func Lookup(m *ebpf.Map, pid uint32, v *ExecveValue) error {
	return m.Lookup(&ExecveKey{Pid: pid}, v)
}

// Put sets the entry of pid in m, the execve_map.
func Put(m *ebpf.Map, pid uint32, v *ExecveValue) error {
	return m.Put(&ExecveKey{Pid: pid}, v)
}
