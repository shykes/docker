package daemon

type Server interface {
	// FIXME: this call is deprecated, the 'logevent' job should be used instead.
	LogEvent(action, id, from string)
	IsRunning() bool // returns true if the server is currently in operation
}
