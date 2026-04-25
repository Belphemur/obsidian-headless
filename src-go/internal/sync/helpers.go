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
