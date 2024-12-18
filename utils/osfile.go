package utils

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
	"strings"
)

func GetMachineID() string {
	var id string
	var err error
	switch runtime.GOOS {
	case "linux":
		id, err = getMachineIDLinux()
	case "windows":
		id, err = getMachineIDWindows()
	case "darwin":
		id = ""
	default:
		err = fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	if err != nil {
		fmt.Println("GetMachineID error:", err)
	}
	return id
}

func getMachineIDLinux() (string, error) {
	// On Linux, use /etc/machine-id
	data, err := ioutil.ReadFile("/etc/machine-id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func getMachineIDWindows() (string, error) {
	// On Windows, use the registry to get the MachineGuid
	cmd := exec.Command("reg", "query", "HKLM\\SOFTWARE\\Microsoft\\Cryptography", "/v", "MachineGuid")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	lastLine := lines[len(lines)-1]
	parts := strings.Split(lastLine, "    ")
	return parts[len(parts)-1], nil
}
