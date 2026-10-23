package config

import "golang.org/x/sys/windows"

func downloadsDirectory() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Downloads, 0)
}
