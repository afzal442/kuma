// +build dev

package controlplane

import (
	"net/http"
	"path/filepath"
	"runtime"
)

var Templates http.FileSystem = http.Dir(TemplatesDir(konvoyctlSrcDir()))

func konvoyctlSrcDir() string {
	_, thisFile, _, _ := runtime.Caller(1)

	thisDir := filepath.Dir(thisFile)

	return filepath.Join(thisDir, "..", "..", "..", "..")
}
