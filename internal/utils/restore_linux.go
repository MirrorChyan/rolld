package utils

import (
	"fmt"
	"os"
	"os/exec"
)

func Restore() {
	fmt.Println("restore terminal")
	cmd := exec.Command("stty", "sane")
	cmd.Stdin = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Println(err)
	}
}
