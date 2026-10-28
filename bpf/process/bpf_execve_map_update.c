// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright Authors of Cilium */

#include "vmlinux.h"
#include "api.h"
#include "bpf_tracing.h"
#include "common.h"
#include "process.h"
#include "config.h"

#define MAX_PIDS 32768

struct update_data {
	__u32 bit;
	__u32 cnt;
	__u32 pids[MAX_PIDS];
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __s32);
	__type(value, struct update_data);
} execve_map_update_data SEC(".maps");

char _license[] __attribute__((section("license"), used)) = "Dual BSD/GPL";

#ifdef __LARGE_BPF_PROG
FUNC_INLINE int
__execve_map_update_idx(struct update_data *data, int idx)
{
	struct execve_map_value *curr;
	__u32 pid;

	asm volatile("%[idx] &= 0x7fff;\n"
		     : [idx] "+r"(idx));
	if (data->cnt == idx)
		return 1;

	pid = data->pids[idx];
	curr = execve_map_get_noinit(pid);
	if (curr)
		lock_and(&curr->bin.mb_bitset, ~(1 << data->bit));
	return 0;
}

FUNC_INLINE void
__execve_map_update(struct update_data *data)
{
	__u32 idx;

	if (CONFIG(ITER_NUM)) {
		bpf_for(idx, 0, MAX_PIDS)
		{
			if (__execve_map_update_idx(data, idx))
				break;
		}
	} else {
		for (idx = 0; idx < 1024; idx++) {
			if (__execve_map_update_idx(data, idx))
				break;
		}
	}
}
#else
FUNC_INLINE void
__execve_map_update(struct update_data *data)
{
}
#endif /* __LARGE_BPF_PROG */

__attribute__((section("socket"), used)) int
execve_map_update(void *ctx)
{
	struct update_data *data;
	__u32 idx = 0;

	/* the task storage execve_map is updated by execve_map_update_task */
	if (EXECVE_MAP_TASK_STORAGE)
		return -1;

	data = map_lookup_elem(&execve_map_update_data, &idx);
	if (!data)
		return -1;
	__execve_map_update(data);
	return 0;
}

#ifdef __V61_BPF_PROG
/* The task storage execve_map entries can only be reached through the tasks,
 * which socket programs can't get, and userspace can't iterate the map to find
 * the pids with the bit set. So this syscall program walks all the processes
 * and clears the bit itself, ignoring data->pids. It returns the number of
 * entries that had the bit set.
 */
__attribute__((section("syscall"), used)) int
execve_map_update_task(void *ctx)
{
	struct execve_map_value *curr;
	struct update_data *data;
	struct task_struct *task;
	__u32 idx = 0;
	__u64 bit;

	if (!EXECVE_MAP_TASK_STORAGE)
		return -1;

	data = map_lookup_elem(&execve_map_update_data, &idx);
	if (!data)
		return -1;

	bit = 1ULL << (data->bit & 63);
	/* Counted in the map value rather than on the stack, so the verifier
	 * does not track it and the iterator loop states can converge.
	 */
	data->cnt = 0;
	bpf_rcu_read_lock();
	bpf_for_each(task, task, 0, BPF_TASK_ITER_ALL_PROCS)
	{
		curr = task_storage_get(&execve_map, task, 0, 0);
		if (curr && (curr->bin.mb_bitset & bit)) {
			lock_and(&curr->bin.mb_bitset, ~bit);
			data->cnt++;
		}
	}
	bpf_rcu_read_unlock();
	return data->cnt;
}
#endif
