// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package base

import (
	"github.com/cilium/tetragon/pkg/sensors"
	"github.com/cilium/tetragon/pkg/sensors/program"
)

// IsExecve returns true if this is a base execve program
func IsExecve(p *program.Program) bool {
	return p.PinName == "event_execve" && p.Policy == sensors.BaseSensorName
}

// IsFork returns true if this is a base fork program, Fork or SchedProcessFork
func IsFork(p *program.Program) bool {
	return (p.PinName == Fork.PinName || p.PinName == SchedProcessFork.PinName) &&
		p.Policy == sensors.BaseSensorName
}

func IsExit(p *program.Program) bool {
	return p.PinName == "event_exit" && p.Policy == sensors.BaseSensorName
}

func IsProcScan(p *program.Program) bool {
	return p.PinName == "proc_scan" && p.Policy == sensors.BaseSensorName
}
