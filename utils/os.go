package utils

import "runtime"

func MacOS() bool {
	return runtime.GOOS == "darwin"
}

func LinuxOS() bool {
	return runtime.GOOS == "linux"
}

func WindowsOS() bool {
	return runtime.GOOS == "windows"
}
