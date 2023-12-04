package astiflow

type Status uint32

// Must be in order of execution
const (
	StatusCreated Status = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusDone
)

func (s Status) String() string {
	switch s {
	case StatusCreated:
		return "created"
	case StatusRunning:
		return "running"
	case StatusStarting:
		return "starting"
	case StatusStopping:
		return "stopping"
	default:
		return "done"
	}
}
