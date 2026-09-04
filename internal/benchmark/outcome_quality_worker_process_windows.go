//go:build windows

package benchmark

import (
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureOutcomeQualityWorkerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func attachOutcomeQualityWorkerProcess(command *exec.Cmd) (func(), func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	closeJob := func() { _ = windows.CloseHandle(job) }

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		closeJob()
		return nil, nil, err
	}

	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		closeJob()
		return nil, nil, err
	}
	assignErr := windows.AssignProcessToJobObject(job, process)
	_ = windows.CloseHandle(process)
	if assignErr != nil {
		closeJob()
		return nil, nil, assignErr
	}

	terminate := func() {
		if err := windows.TerminateJobObject(job, 1); err != nil {
			terminateOutcomeQualityWorkerCommand(command)
		}
	}
	return terminate, closeJob, nil
}

func terminateOutcomeQualityWorkerCommand(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	kill.Env = outcomeQualityWorkerEnvironment()
	_ = kill.Run()
}
