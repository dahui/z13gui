//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_tracing.h>

#define EAGAIN  11
#define MAY_READ 4
#define MAJOR(dev) ((unsigned int)((dev) >> 20))

// blocked_pids: PIDs whose hidraw reads should return -EAGAIN.
struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 64);
	__type(key, __u32);
	__type(value, __u8);
} blocked_pids SEC(".maps");

// hidraw_config: index 0 = hidraw character device major number.
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u32);
} hidraw_config SEC(".maps");

SEC("lsm/file_permission")
int BPF_PROG(hidraw_block, struct file *file, int mask, int ret)
{
	if (ret != 0)
		return ret;

	if (!(mask & MAY_READ))
		return 0;

	__u32 pid = bpf_get_current_pid_tgid() >> 32;
	if (!bpf_map_lookup_elem(&blocked_pids, &pid))
		return 0;

	dev_t rdev = BPF_CORE_READ(file, f_inode, i_rdev);
	__u32 major = MAJOR(rdev);

	__u32 key = 0;
	__u32 *hidraw_major = bpf_map_lookup_elem(&hidraw_config, &key);
	if (!hidraw_major || major != *hidraw_major)
		return 0;

	return -EAGAIN;
}

char LICENSE[] SEC("license") = "GPL";
