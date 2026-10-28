// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package execvemap

import (
	"github.com/cilium/tetragon/pkg/api/processapi"
)

const (
	// HashMapName is the name of the hash execve_map, keyed by pid.
	HashMapName = "execve_map"
	// TaskStorageMapName is the name of the task storage execve_map.
	TaskStorageMapName = "execve_map_ts"
)

type ExecveKey struct {
	Pid uint32
}

type ExecveValue struct {
	Process      processapi.MsgExecveKey    `align:"key"`
	Parent       processapi.MsgExecveKey    `align:"pkey"`
	Flags        uint32                     `align:"flags"`
	Nspid        uint32                     `align:"nspid"`
	Namespaces   processapi.MsgNamespaces   `align:"ns"`
	Capabilities processapi.MsgCapabilities `align:"caps"`
	Binary       processapi.Binary          `align:"bin"`
	Args         processapi.Args            `align:"args"`
}
