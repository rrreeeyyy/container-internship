// +build linux
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func Run() {
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWUSER |
			syscall.CLONE_NEWUTS,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func InitContainer() error {
	if err := syscall.Sethostname([]byte("container")); err != nil {
		return fmt.Errorf("Setting hostname failed: %w", err)
	}
	if err := syscall.Mount("proc", "/root/rootfs/proc", "proc", uintptr(syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV), ""); err != nil {
		return fmt.Errorf("Proc mount failed: %w", err)
	}
	if err := os.Chdir("/root"); err != nil {
		return fmt.Errorf("Chdir /root failed: %w", err)
	}
	if err := syscall.Mount("rootfs", "/root/rootfs", "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("Rootfs bind mount failed: %w", err)
	}
	if err := os.MkdirAll("/root/rootfs/oldrootfs", 0700); err != nil {
		return fmt.Errorf("Oldrootfs create failed: %w", err)
	}
	if err := syscall.PivotRoot("rootfs", "/root/rootfs/oldrootfs"); err != nil {
		return fmt.Errorf("PivotRoot failed: %w", err)
	}
	if err := syscall.Unmount("/oldrootfs", syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("Oldrootfs umount failed: %w", err)
	}
	if err := os.RemoveAll("/oldrootfs"); err != nil {
		return fmt.Errorf("Remove oldrootfs failed: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("Chdir failed: %w", err)
	}
	if err := syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ()); err != nil {
		return fmt.Errorf("Exec failed: %w", err)
	}
	return nil
}

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s run\n", os.Args[0])
	os.Exit(2)
}

func main() {
	if len(os.Args) <= 1 {
		Usage()
	}
	switch os.Args[1] {
	case "run":
		Run()
	case "init":
		if err := InitContainer(); err != nil {
			fmt.Fprintf(os.Stderr, "%+v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	default:
		Usage()
	}
}
