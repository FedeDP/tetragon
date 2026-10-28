// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Tetragon

package mapmetrics

import (
	"github.com/cilium/tetragon/pkg/metrics"
	"github.com/cilium/tetragon/pkg/metrics/consts"
)

var MapLabel = metrics.ConstrainedLabel{
	Name: "map",
	// These are maps whose usage we monitor, not maps from which we read
	// metrics. Metrics are read from separate maps suffixed with "_stats".
	Values: []string{"execve_map", "tg_execve_joined_info_map"},
}

// Aliases maps the name of a map to the one it's reported as, and whose
// "_stats" map it uses: the task storage execve_map is reported as execve_map.
var Aliases = map[string]string{
	"execve_map_ts": "execve_map",
}

var (
	MapSize = metrics.MustNewCustomGauge(metrics.NewOpts(
		consts.MetricsNamespace, "", "map_entries",
		"The total number of in-use entries per map.",
		nil, []metrics.ConstrainedLabel{MapLabel}, nil,
	))
	// On v5.11 and v6.1 bpf objects, this will always be 0 for tg_execve_joined_info_map
	// since it is stored as task storage, and so is execve_map when it's a task storage
	// map.
	MapCapacity = metrics.MustNewCustomGauge(metrics.NewOpts(
		consts.MetricsNamespace, "", "map_capacity",
		"Capacity of a BPF map. Expected to be constant.",
		nil, []metrics.ConstrainedLabel{MapLabel}, nil,
	))
	MapErrorsUpdate = metrics.MustNewCustomGauge(metrics.NewOpts(
		consts.MetricsNamespace, "", "map_errors_update_total",
		"The number of failed updates per map.",
		nil, []metrics.ConstrainedLabel{MapLabel}, nil,
	))
	MapErrorsDelete = metrics.MustNewCustomGauge(metrics.NewOpts(
		consts.MetricsNamespace, "", "map_errors_delete_total",
		"The number of failed deletes per map.",
		nil, []metrics.ConstrainedLabel{MapLabel}, nil,
	))
)
