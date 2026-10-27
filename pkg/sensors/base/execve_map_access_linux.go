// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package base

import (
	"github.com/cilium/ebpf"

	"github.com/cilium/tetragon/pkg/sensors/exec/execvemap"
)

func init() {
	execvemap.RegisterAccessProgramAtInit(func() *ebpf.Program {
		return ExecveMapAccess.Prog
	})
}
