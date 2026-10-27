// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright Authors of Tetragon */

#include "vmlinux.h"
#include "api.h"
#include "bpf_tracing.h"

#include "compiler.h"
#include "common.h"
#include "errno.h"
#include "process.h"

char _license[] __attribute__((section("license"), used)) = "Dual BSD/GPL";

#define EXECVE_MAP_ACCESS_LOOKUP 0
#define EXECVE_MAP_ACCESS_UPDATE 1

/* The verifier only allows direct context accesses at a constant offset from
 * the unmodified context pointer, so the value comes first: copying it doesn't
 * need to move the pointer.
 */
struct execve_map_access {
	struct execve_map_value value;
	__u32 op;
	__u32 pid;
};

/*
 * Userspace access to the task storage execve_map entries.
 *
 * Userspace can only reach task storage through a pidfd, which is bound to
 * the pid namespace of the caller, while tetragon usually runs without
 * hostPID. bpf_task_from_pid() instead looks up the pid in the initial pid
 * namespace, the one of the pids in execve_map. The program is run with
 * BPF_PROG_TEST_RUN, with a struct execve_map_access as the context: the
 * entry of the process is copied to or from ctx->value.
 */
__attribute__((section("syscall"), used)) int
execve_map_access(struct execve_map_access *ctx)
{
	struct execve_map_value *curr;
	struct task_struct *task;
	__u32 zero = 0;
	int err = 0;

	if (!EXECVE_MAP_TASK_STORAGE)
		return -EINVAL;

	if (!ctx->pid) {
		curr = map_lookup_elem(&execve_map_kernel, &zero);
		if (!curr)
			return -ENOENT;
		if (ctx->op == EXECVE_MAP_ACCESS_UPDATE)
			__bpf_memcpy_builtin(curr, &ctx->value, sizeof(*curr));
		else
			__bpf_memcpy_builtin(&ctx->value, curr, sizeof(*curr));
		return 0;
	}

	task = bpf_task_from_pid(ctx->pid);
	if (!task)
		return -ESRCH;

	if (ctx->op == EXECVE_MAP_ACCESS_UPDATE) {
		curr = task_storage_get(&execve_map, task, 0, BPF_LOCAL_STORAGE_GET_F_CREATE);
		if (curr)
			__bpf_memcpy_builtin(curr, &ctx->value, sizeof(*curr));
		else
			err = -ENOMEM;
	} else {
		curr = task_storage_get(&execve_map, task, 0, 0);
		if (curr)
			__bpf_memcpy_builtin(&ctx->value, curr, sizeof(*curr));
		else
			err = -ENOENT;
	}

	bpf_task_release(task);
	return err;
}
