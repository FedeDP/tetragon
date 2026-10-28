// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright Authors of Tetragon */

#include "vmlinux.h"
#include "api.h"
#include "bpf_tracing.h"

#include "compiler.h"
#include "common.h"
#include "process.h"

char _license[] __attribute__((section("license"), used)) = "Dual BSD/GPL";

/*
 * With task storage, the execve_map entry of a process lives on its thread
 * group leader. When a thread other than the leader calls execve, de_thread()
 * makes it the new leader and releases the old one, together with the entry,
 * before the exec hooks run.
 *
 * begin_new_exec() calls de_thread(), so copy the entry to the calling thread
 * here, where the exec hooks find it once the thread is the leader. If the
 * exec fails before de_thread(), the copy is left unused on the thread and it
 * is released with it. Either way only one of the two entries gets deleted at
 * exit, so the copy is not counted in execve_map_stats.
 */
__attribute__((section("kprobe/begin_new_exec"), used)) int
BPF_KPROBE(tg_kp_begin_new_exec)
{
	struct execve_map_value *curr, *value;
	struct task_struct *task, *leader;

	if (!EXECVE_MAP_TASK_STORAGE)
		return 0;

	task = (struct task_struct *)get_current_task_btf();
	if (*_(&task->pid) == *_(&task->tgid))
		return 0;

	leader = *_(&task->group_leader);
	if (!leader)
		return 0;
	curr = task_storage_get(&execve_map_ts, leader, 0, 0);
	if (!curr)
		return 0;

	value = task_storage_get(&execve_map_ts, task, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
	if (!value) {
		STATS_INC(execve_map_stats, EUPDATE);
		return 0;
	}
	__bpf_memcpy_builtin(value, curr, sizeof(*value));
	return 0;
}
