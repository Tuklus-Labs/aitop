//go:build darwin && cgo

package proc

/*
#cgo LDFLAGS: -lproc
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <mach/mach_time.h>
#include <mach/vm_statistics.h>
#include <libproc.h>
#include <pthread.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc.h>
#include <sys/proc_info.h>
#include <sys/sysctl.h>
#include <sys/time.h>
#include <sys/types.h>
#include <unistd.h>

static pthread_once_t aitop_mach_once = PTHREAD_ONCE_INIT;
static host_t aitop_host_port;
static mach_timebase_info_data_t aitop_timebase;

static void aitop_init_mach(void) {
	aitop_host_port = mach_host_self();
	mach_timebase_info(&aitop_timebase);
}

static host_t aitop_host(void) {
	pthread_once(&aitop_mach_once, aitop_init_mach);
	return aitop_host_port;
}

static uint64_t aitop_mach_to_ns(uint64_t ticks) {
	pthread_once(&aitop_mach_once, aitop_init_mach);
	if (aitop_timebase.denom == 0) {
		return 0;
	}
	__uint128_t ns = (__uint128_t)ticks * aitop_timebase.numer / aitop_timebase.denom;
	if (ns > (__uint128_t)UINT64_MAX) {
		return UINT64_MAX;
	}
	return (uint64_t)ns;
}

static int aitop_list_pids(int32_t *out, int cap) {
	int bytes = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
	if (bytes <= 0) {
		return -1;
	}
	int need = (bytes + (int)sizeof(int32_t) - 1) / (int)sizeof(int32_t);
	if (out == NULL || cap < need) {
		return need;
	}
	int got = proc_listpids(PROC_ALL_PIDS, 0, out, cap * (int)sizeof(int32_t));
	if (got <= 0) {
		return -1;
	}
	return (got + (int)sizeof(int32_t) - 1) / (int)sizeof(int32_t);
}

static void aitop_copy_string(char *dst, size_t cap, const char *src, size_t max) {
	if (dst == NULL || cap == 0) {
		return;
	}
	size_t n = 0;
	while (n < max && src[n] != '\0' && n+1 < cap) {
		n++;
	}
	if (n != 0) {
		memcpy(dst, src, n);
	}
	dst[n] = '\0';
}

static int aitop_proc_info(int pid, int32_t *ppid, uint64_t *start_us,
    uint64_t *resident, uint64_t *user_ns, uint64_t *system_ns,
    uint32_t *status, int32_t *running, char *comm, size_t comm_cap) {
	struct proc_taskallinfo info;
	memset(&info, 0, sizeof(info));
	int got = proc_pidinfo(pid, PROC_PIDTASKALLINFO, 0, &info, sizeof(info));
	if (got < (int)sizeof(info)) {
		return -1;
	}
	pthread_once(&aitop_mach_once, aitop_init_mach);
	if (aitop_timebase.denom == 0) {
		return -1;
	}
	if (ppid != NULL) {
		*ppid = (int32_t)info.pbsd.pbi_ppid;
	}
	if (start_us != NULL) {
		*start_us = info.pbsd.pbi_start_tvsec * UINT64_C(1000000) + info.pbsd.pbi_start_tvusec;
	}
	if (resident != NULL) {
		*resident = info.ptinfo.pti_resident_size;
	}
	if (user_ns != NULL) {
		*user_ns = aitop_mach_to_ns(info.ptinfo.pti_total_user);
	}
	if (system_ns != NULL) {
		*system_ns = aitop_mach_to_ns(info.ptinfo.pti_total_system);
	}
	if (status != NULL) {
		*status = info.pbsd.pbi_status;
	}
	if (running != NULL) {
		*running = info.ptinfo.pti_numrunning;
	}
	aitop_copy_string(comm, comm_cap, info.pbsd.pbi_comm, sizeof(info.pbsd.pbi_comm));
	return 0;
}

static int aitop_proc_paths(int pid, char *exe, size_t exe_cap, char *cwd, size_t cwd_cap) {
	char path[MAXPATHLEN];
	memset(path, 0, sizeof(path));
	int n = proc_pidpath(pid, path, sizeof(path));
	if (n > 0) {
		aitop_copy_string(exe, exe_cap, path, (size_t)n);
	}

	struct proc_vnodepathinfo vnode;
	memset(&vnode, 0, sizeof(vnode));
	int got = proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0, &vnode, sizeof(vnode));
	if (got >= (int)sizeof(vnode)) {
		aitop_copy_string(cwd, cwd_cap, vnode.pvi_cdir.vip_path, sizeof(vnode.pvi_cdir.vip_path));
	}
	return 0;
}

// KERN_PROCARGS2 contains argc, the executable path, argc argv strings, and
// then the environment. Copy only the argv portion into a NUL-separated
// buffer, preserving spaces and empty arguments while excluding env strings.
static int aitop_proc_args(int pid, char *out, size_t cap) {
	int mib[3] = {CTL_KERN, KERN_PROCARGS2, pid};
	size_t size = 0;
	if (sysctl(mib, 3, NULL, &size, NULL, 0) != 0 || size < sizeof(int)) {
		return -1;
	}
	char *raw = (char *)malloc(size);
	if (raw == NULL) {
		return -1;
	}
	if (sysctl(mib, 3, raw, &size, NULL, 0) != 0 || size < sizeof(int)) {
		free(raw);
		return -1;
	}
	int argc = 0;
	memcpy(&argc, raw, sizeof(argc));
	if (argc < 0 || argc > 4096) {
		free(raw);
		return -1;
	}
	char *p = raw + sizeof(int);
	char *end = raw + size;
	while (p < end && *p != '\0') {
		p++;
	}
	while (p < end && *p == '\0') {
		p++;
	}
	size_t used = 0;
	for (int i = 0; i < argc; i++) {
		if (p >= end) {
			free(raw);
			return -1;
		}
		char *start = p;
		while (p < end && *p != '\0') {
			p++;
		}
		size_t len = (size_t)(p - start);
		if (out != NULL && used + len + 1 <= cap) {
			memcpy(out + used, start, len);
			out[used + len] = '\0';
		}
		used += len + 1;
		if (p >= end) {
			free(raw);
			return -1;
		}
		p++;
	}
	free(raw);
	if (used > (size_t)INT32_MAX) {
		return -1;
	}
	return (int)used;
}

static uint64_t aitop_boot_time_us(void) {
	struct timeval tv;
	memset(&tv, 0, sizeof(tv));
	size_t size = sizeof(tv);
	if (sysctlbyname("kern.boottime", &tv, &size, NULL, 0) != 0) {
		return 0;
	}
	if (tv.tv_sec < 0 || tv.tv_usec < 0) {
		return 0;
	}
	return (uint64_t)tv.tv_sec * UINT64_C(1000000) + (uint64_t)tv.tv_usec;
}

static int aitop_host_cpu(uint64_t *busy, uint64_t *total) {
	host_cpu_load_info_data_t info;
	memset(&info, 0, sizeof(info));
	mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
	if (host_statistics(aitop_host(), HOST_CPU_LOAD_INFO,
	    (host_info_t)&info, &count) != KERN_SUCCESS) {
		return -1;
	}
	uint64_t user = info.cpu_ticks[CPU_STATE_USER];
	uint64_t system = info.cpu_ticks[CPU_STATE_SYSTEM];
	uint64_t nice = info.cpu_ticks[CPU_STATE_NICE];
	uint64_t idle = info.cpu_ticks[CPU_STATE_IDLE];
	if (busy != NULL) {
		*busy = user + system + nice;
	}
	if (total != NULL) {
		*total = user + system + nice + idle;
	}
	return 0;
}

static int aitop_host_memory(uint64_t *total, uint64_t *available) {
	uint64_t mem = 0;
	size_t mem_size = sizeof(mem);
	if (sysctlbyname("hw.memsize", &mem, &mem_size, NULL, 0) != 0) {
		return -1;
	}
	vm_statistics64_data_t vm;
	memset(&vm, 0, sizeof(vm));
	mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
	if (host_statistics64(aitop_host(), HOST_VM_INFO64,
	    (host_info64_t)&vm, &count) != KERN_SUCCESS) {
		return -1;
	}
	// vm_statistics64 documents speculative pages as already included in
	// free_count. Adding them again overstates available memory.
	uint64_t pages = (uint64_t)vm.free_count + (uint64_t)vm.inactive_count;
	if (total != NULL) {
		*total = mem;
	}
	if (available != NULL) {
		*available = pages * (uint64_t)getpagesize();
	}
	return 0;
}

static char aitop_state(uint32_t state, int32_t running) {
	switch (state) {
	case SRUN:
		return running > 0 ? 'R' : '?';
	case SSLEEP:
		return 'S';
	case SSTOP:
		return 'T';
	case SZOMB:
		return 'Z';
	default:
		return '?';
	}
}
*/
import "C"

import (
	"errors"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

var errNativeUnavailable = errors.New("Darwin process collector unavailable")

func nativeRoot(root string) bool { return root == "" || root == "/proc" }

var darwinBoot atomic.Uint64

func nativeBootTimeUS() uint64 {
	if boot := darwinBoot.Load(); boot != 0 {
		return boot
	}
	boot := uint64(C.aitop_boot_time_us())
	if boot != 0 {
		darwinBoot.CompareAndSwap(0, boot)
	}
	return boot
}

func nativeClkTck() (int64, bool) { return 100, true }

func nativeStartTicks(startUS uint64) uint64 {
	boot := nativeBootTimeUS()
	if boot == 0 || startUS <= boot {
		return 0
	}
	return (startUS - boot) * uint64(nativeClkTckValue) / 1_000_000
}

const nativeClkTckValue int64 = 100

func cString(buf []byte) string {
	if i := strings.IndexByte(string(buf), 0); i >= 0 {
		return string(buf[:i])
	}
	return string(buf)
}

func nativeArgs(pid int32) []string {
	n := C.aitop_proc_args(C.int(pid), nil, 0)
	if n <= 0 {
		return nil
	}
	raw := make([]byte, int(n))
	got := C.aitop_proc_args(C.int(pid), (*C.char)(unsafe.Pointer(&raw[0])), C.size_t(len(raw)))
	if got <= 0 || int(got) > len(raw) {
		return nil
	}
	return parseNativeCmdline(raw[:got])
}

func nativePaths(pid int32) (exe, cwd string) {
	exeBuf := make([]byte, 4096)
	cwdBuf := make([]byte, 4096)
	if C.aitop_proc_paths(C.int(pid), (*C.char)(unsafe.Pointer(&exeBuf[0])), C.size_t(len(exeBuf)),
		(*C.char)(unsafe.Pointer(&cwdBuf[0])), C.size_t(len(cwdBuf))) != 0 {
		return "", ""
	}
	return cString(exeBuf), cString(cwdBuf)
}

func nativeFullProcess(pid int32) (types.Process, bool) {
	var ppid C.int32_t
	var startUS, resident, userNS, systemNS C.uint64_t
	var status C.uint32_t
	var running C.int32_t
	commBuf := make([]byte, 256)
	if C.aitop_proc_info(C.int(pid), &ppid, &startUS, &resident, &userNS, &systemNS,
		&status, &running, (*C.char)(unsafe.Pointer(&commBuf[0])), C.size_t(len(commBuf))) != 0 {
		return types.Process{}, false
	}
	p := types.Process{
		PID:       pid,
		PPID:      int32(ppid),
		StartTime: nativeStartTicks(uint64(startUS)),
		Comm:      cString(commBuf),
		RSS:       uint64(resident),
		Utime:     uint64(userNS) / 10_000_000,
		Stime:     uint64(systemNS) / 10_000_000,
		State:     byte(C.aitop_state(status, running)),
	}
	if !Candidate(p.Comm) {
		return p, true
	}
	p.Cmdline = nativeArgs(pid)
	if !fullComms[p.Comm] && !fuzzyAgent(p.Comm) {
		return p, true
	}
	p.Exe, p.CWD = nativePaths(pid)
	return p, true
}

func walkNative() ([]types.Process, error) {
	need := int(C.aitop_list_pids(nil, 0))
	if need <= 0 {
		return nil, errNativeUnavailable
	}
	for attempt := 0; attempt < 3; attempt++ {
		pids := make([]C.int32_t, need)
		got := int(C.aitop_list_pids(&pids[0], C.int(len(pids))))
		if got < 0 {
			return nil, errNativeUnavailable
		}
		if got > len(pids) {
			need = got
			continue
		}
		out := make([]types.Process, 0, got)
		for _, cpid := range pids[:got] {
			pid := int32(cpid)
			if pid <= 0 {
				continue
			}
			if p, ok := nativeFullProcess(pid); ok {
				out = append(out, p)
			}
		}
		return out, nil
	}
	return nil, errNativeUnavailable
}

func nativeInitHost(h *Host) {
	h.btime = int64(nativeBootTimeUS() / 1_000_000)
	var total C.uint64_t
	var available C.uint64_t
	if C.aitop_host_memory(&total, &available) == 0 {
		h.memTot = uint64(total)
	}
}

func nativeHostSample(h *Host) HostSample {
	s := HostSample{
		NumCPU:   h.ncpu,
		BootTime: h.btime,
		MemTotal: h.memTot,
		ClkTck:   h.clk,
	}
	if boot := nativeBootTimeUS(); boot != 0 {
		now := time.Now().UnixMicro()
		if now > int64(boot) {
			s.Uptime = float64(now-int64(boot)) / 1_000_000
		}
	}
	var busy, total C.uint64_t
	if C.aitop_host_cpu(&busy, &total) == 0 {
		cur := cpuTotals{busy: uint64(busy), total: uint64(total)}
		if h.havePr {
			dt := int64(cur.total) - int64(h.prev.total)
			db := int64(cur.busy) - int64(h.prev.busy)
			if dt > 0 && db >= 0 {
				s.CPUBusyPct = 100 * float64(db) / float64(dt)
				s.CPUKnown = true
			}
		}
		h.prev = cur
		h.havePr = true
	}
	var available C.uint64_t
	if C.aitop_host_memory(nil, &available) == 0 {
		s.MemAvail = uint64(available)
	}
	return s
}
