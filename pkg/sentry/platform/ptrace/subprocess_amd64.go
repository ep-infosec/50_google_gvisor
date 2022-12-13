// Copyright 2018 The gVisor Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build amd64
// +build amd64

package ptrace

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
	"gvisor.dev/gvisor/pkg/abi/linux"
	"gvisor.dev/gvisor/pkg/seccomp"
	"gvisor.dev/gvisor/pkg/sentry/arch"
)

const (
	// maximumUserAddress is the largest possible user address.
	maximumUserAddress = 0x7ffffffff000

	// stubInitAddress is the initial attempt link address for the stub.
	stubInitAddress = 0x7fffffff0000

	// initRegsRipAdjustment is the size of the syscall instruction.
	initRegsRipAdjustment = 2
)

// resetSysemuRegs sets up emulation registers.
//
// This should be called prior to calling sysemu.
func (t *thread) resetSysemuRegs(regs *arch.Registers) {
	regs.Cs = t.initRegs.Cs
	regs.Ss = t.initRegs.Ss
	regs.Ds = t.initRegs.Ds
	regs.Es = t.initRegs.Es
	regs.Fs = t.initRegs.Fs
	regs.Gs = t.initRegs.Gs
}

// createSyscallRegs sets up syscall registers.
//
// This should be called to generate registers for a system call.
func createSyscallRegs(initRegs *arch.Registers, sysno uintptr, args ...arch.SyscallArgument) arch.Registers {
	// Copy initial registers.
	regs := *initRegs

	// Set our syscall number.
	regs.Rax = uint64(sysno)
	if len(args) >= 1 {
		regs.Rdi = args[0].Uint64()
	}
	if len(args) >= 2 {
		regs.Rsi = args[1].Uint64()
	}
	if len(args) >= 3 {
		regs.Rdx = args[2].Uint64()
	}
	if len(args) >= 4 {
		regs.R10 = args[3].Uint64()
	}
	if len(args) >= 5 {
		regs.R8 = args[4].Uint64()
	}
	if len(args) >= 6 {
		regs.R9 = args[5].Uint64()
	}

	return regs
}

// isSingleStepping determines if the registers indicate single-stepping.
func isSingleStepping(regs *arch.Registers) bool {
	return (regs.Eflags & arch.X86TrapFlag) != 0
}

// updateSyscallRegs updates registers after finishing sysemu.
func updateSyscallRegs(regs *arch.Registers) {
	// Ptrace puts -ENOSYS in rax on syscall-enter-stop.
	regs.Rax = regs.Orig_rax
}

// syscallReturnValue extracts a sensible return from registers.
func syscallReturnValue(regs *arch.Registers) (uintptr, error) {
	rval := int64(regs.Rax)
	if rval < 0 {
		return 0, unix.Errno(-rval)
	}
	return uintptr(rval), nil
}

func dumpRegs(regs *arch.Registers) string {
	var m strings.Builder

	fmt.Fprintf(&m, "Registers:\n")
	fmt.Fprintf(&m, "\tR15\t = %016x\n", regs.R15)
	fmt.Fprintf(&m, "\tR14\t = %016x\n", regs.R14)
	fmt.Fprintf(&m, "\tR13\t = %016x\n", regs.R13)
	fmt.Fprintf(&m, "\tR12\t = %016x\n", regs.R12)
	fmt.Fprintf(&m, "\tRbp\t = %016x\n", regs.Rbp)
	fmt.Fprintf(&m, "\tRbx\t = %016x\n", regs.Rbx)
	fmt.Fprintf(&m, "\tR11\t = %016x\n", regs.R11)
	fmt.Fprintf(&m, "\tR10\t = %016x\n", regs.R10)
	fmt.Fprintf(&m, "\tR9\t = %016x\n", regs.R9)
	fmt.Fprintf(&m, "\tR8\t = %016x\n", regs.R8)
	fmt.Fprintf(&m, "\tRax\t = %016x\n", regs.Rax)
	fmt.Fprintf(&m, "\tRcx\t = %016x\n", regs.Rcx)
	fmt.Fprintf(&m, "\tRdx\t = %016x\n", regs.Rdx)
	fmt.Fprintf(&m, "\tRsi\t = %016x\n", regs.Rsi)
	fmt.Fprintf(&m, "\tRdi\t = %016x\n", regs.Rdi)
	fmt.Fprintf(&m, "\tOrig_rax = %016x\n", regs.Orig_rax)
	fmt.Fprintf(&m, "\tRip\t = %016x\n", regs.Rip)
	fmt.Fprintf(&m, "\tCs\t = %016x\n", regs.Cs)
	fmt.Fprintf(&m, "\tEflags\t = %016x\n", regs.Eflags)
	fmt.Fprintf(&m, "\tRsp\t = %016x\n", regs.Rsp)
	fmt.Fprintf(&m, "\tSs\t = %016x\n", regs.Ss)
	fmt.Fprintf(&m, "\tFs_base\t = %016x\n", regs.Fs_base)
	fmt.Fprintf(&m, "\tGs_base\t = %016x\n", regs.Gs_base)
	fmt.Fprintf(&m, "\tDs\t = %016x\n", regs.Ds)
	fmt.Fprintf(&m, "\tEs\t = %016x\n", regs.Es)
	fmt.Fprintf(&m, "\tFs\t = %016x\n", regs.Fs)
	fmt.Fprintf(&m, "\tGs\t = %016x\n", regs.Gs)

	return m.String()
}

// adjustInitregsRip adjust the current register RIP value to
// be just before the system call instruction excution
func (t *thread) adjustInitRegsRip() {
	t.initRegs.Rip -= initRegsRipAdjustment
}

// Pass the expected PPID to the child via R15 when creating stub process.
func initChildProcessPPID(initregs *arch.Registers, ppid int32) {
	initregs.R15 = uint64(ppid)
	// Rbx has to be set to 1 when creating stub process.
	initregs.Rbx = 1
}

// patchSignalInfo patches the signal info to account for hitting the seccomp
// filters from vsyscall emulation, specified below. We allow for SIGSYS as a
// synchronous trap, but patch the structure to appear like a SIGSEGV with the
// Rip as the faulting address.
//
// Note that this should only be called after verifying that the signalInfo has
// been generated by the kernel.
func patchSignalInfo(regs *arch.Registers, signalInfo *linux.SignalInfo) {
	if linux.Signal(signalInfo.Signo) == linux.SIGSYS {
		signalInfo.Signo = int32(linux.SIGSEGV)

		// Unwind the kernel emulation, if any has occurred. A SIGSYS is delivered
		// with the si_call_addr field pointing to the current RIP. This field
		// aligns with the si_addr field for a SIGSEGV, so we don't need to touch
		// anything there. We do need to unwind emulation however, so we set the
		// instruction pointer to the faulting value, and "unpop" the stack.
		regs.Rip = signalInfo.Addr()
		regs.Rsp -= 8
	}
}

// enableCpuidFault enables cpuid-faulting.
//
// This may fail on older kernels or hardware, so we just disregard the result.
// Host CPUID will be enabled.
//
// This is safe to call in an afterFork context.
//
//go:norace
//go:nosplit
func enableCpuidFault() {
	unix.RawSyscall6(unix.SYS_ARCH_PRCTL, linux.ARCH_SET_CPUID, 0, 0, 0, 0, 0)
}

// appendArchSeccompRules append architecture specific seccomp rules when creating BPF program.
// Ref attachedThread() for more detail.
func appendArchSeccompRules(rules []seccomp.RuleSet, defaultAction linux.BPFAction) []seccomp.RuleSet {
	rules = append(rules,
		// Rules for trapping vsyscall access.
		seccomp.RuleSet{
			Rules: seccomp.SyscallRules{
				unix.SYS_GETTIMEOFDAY: {},
				unix.SYS_TIME:         {},
				unix.SYS_GETCPU:       {}, // SYS_GETCPU was not defined in package syscall on amd64.
			},
			Action:   linux.SECCOMP_RET_TRAP,
			Vsyscall: true,
		})
	if defaultAction != linux.SECCOMP_RET_ALLOW {
		rules = append(rules,
			seccomp.RuleSet{
				Rules: seccomp.SyscallRules{
					unix.SYS_ARCH_PRCTL: []seccomp.Rule{
						{seccomp.EqualTo(linux.ARCH_SET_CPUID), seccomp.EqualTo(0)},
					},
				},
				Action: linux.SECCOMP_RET_ALLOW,
			})
	}
	return rules
}

// probeSeccomp returns true iff seccomp is run after ptrace notifications,
// which is generally the case for kernel version >= 4.8. This check is dynamic
// because kernels have be backported behavior.
//
// See createStub for more information.
//
// Precondition: the runtime OS thread must be locked.
func probeSeccomp() bool {
	// Create a completely new, destroyable process.
	t, err := attachedThread(0, linux.SECCOMP_RET_ERRNO)
	if err != nil {
		panic(fmt.Sprintf("seccomp probe failed: %v", err))
	}
	defer t.destroy()

	// Set registers to the yield system call. This call is not allowed
	// by the filters specified in the attachThread function.
	regs := createSyscallRegs(&t.initRegs, unix.SYS_SCHED_YIELD)
	if err := t.setRegs(&regs); err != nil {
		panic(fmt.Sprintf("ptrace set regs failed: %v", err))
	}

	for {
		// Attempt an emulation.
		if _, _, errno := unix.RawSyscall6(unix.SYS_PTRACE, unix.PTRACE_SYSEMU, uintptr(t.tid), 0, 0, 0, 0); errno != 0 {
			panic(fmt.Sprintf("ptrace syscall-enter failed: %v", errno))
		}

		sig := t.wait(stopped)
		if sig == (syscallEvent | unix.SIGTRAP) {
			// Did the seccomp errno hook already run? This would
			// indicate that seccomp is first in line and we're
			// less than 4.8.
			if err := t.getRegs(&regs); err != nil {
				panic(fmt.Sprintf("ptrace get-regs failed: %v", err))
			}
			if _, err := syscallReturnValue(&regs); err == nil {
				// The seccomp errno mode ran first, and reset
				// the error in the registers.
				return false
			}
			// The seccomp hook did not run yet, and therefore it
			// is safe to use RET_KILL mode for dispatched calls.
			return true
		}
	}
}

func (s *subprocess) arm64SyscallWorkaround(t *thread, regs *arch.Registers) {
}
