package core

import (
	"fmt"
	"os"
)

func StartGroup(message string) {
	fmt.Println("::group::" + message)
}

func EndGroup() {
	fmt.Println("::endgroup::")
}

func Debug(msg string, args ...any) {
	if os.Getenv("RUNNER_DEBUG") == "1" {
		fmt.Println("##[debug]" + fmt.Sprintf(msg, args...))
	}
}

func DirExists(path string, required bool) error {
	if path == "" {
		return fmt.Errorf("path argument must not be empty")
	}
	if s, err := os.Stat(path); err != nil {
		if required {
			return fmt.Errorf("directory '%s' does not exist: %v", err)
		}
		return nil
	} else if s.IsDir() {
		return nil
	} else {
		return fmt.Errorf("expected '%s' to be a directory but it is a file")
	}
}
