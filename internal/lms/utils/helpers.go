package utils

import (
	"os"
	"strings"
)

// ProjectRootDir finds project root directory as absolute path
func ProjectRootDir() string {
	// fixme(maksym): an ugly solution to find working directory
	// needs to be refactored to something more robust
	cwd, _ := os.Getwd()
	cwd = cwd[:strings.LastIndex(cwd, "/fiskil-lms")+len("/fiskil-lms")]

	return cwd
}
