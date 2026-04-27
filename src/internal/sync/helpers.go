package sync

import "os"

func (e *Engine) deviceName() string {
	if e.Config.DeviceName != "" {
		return e.Config.DeviceName
	}
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

func (e *Engine) configDir() string {
	return e.Config.ConfigDir
}

// ignoreList returns the base list of paths to ignore during local scans
// and file watching, including the sync lock directory.
func (e *Engine) ignoreList() []string {
	return append([]string{e.configDir() + "/.sync.lock"}, e.Config.IgnoreFolders...)
}
