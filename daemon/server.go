package daemon

type Server interface {
	LogEvent(action, id, from string)
	IsRunning() bool // returns true if the server is currently in operation
}
