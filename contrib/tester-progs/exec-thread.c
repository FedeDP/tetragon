#define _GNU_SOURCE
#include <stdio.h>
#include <pthread.h>
#include <unistd.h>

// thread execs /bin/sh while the thread group leader is still running, so the
// exec has to take over the leader (see de_thread() in the kernel)
void *thread(void *arg)
{
	char *argv[] = {"/bin/sh", "-c", "echo exec from a thread", NULL};

	execve("/bin/sh", argv, NULL);
	perror("exec");
	_exit(1);
}

int main(int argc, char **argv)
{
	pthread_t tid;

	if (pthread_create(&tid, NULL, thread, NULL)) {
		perror("pthread_create");
		return 1;
	}
	// the exec kills this thread
	pthread_join(tid, NULL);
	return 1;
}
