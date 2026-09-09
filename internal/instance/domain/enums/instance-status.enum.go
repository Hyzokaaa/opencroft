package enums

type InstanceStatus string

const (
	StatusRunning InstanceStatus = "running"
	StatusStopped InstanceStatus = "stopped"
	StatusFrozen  InstanceStatus = "frozen"
	StatusError   InstanceStatus = "error"
	StatusUnknown InstanceStatus = "unknown"
)

func ParseStatus(raw string) InstanceStatus {
	switch raw {
	case "RUNNING", "Running", "running":
		return StatusRunning
	case "STOPPED", "Stopped", "stopped":
		return StatusStopped
	case "FROZEN", "Frozen", "frozen":
		return StatusFrozen
	case "ERROR", "Error", "error":
		return StatusError
	default:
		return StatusUnknown
	}
}
