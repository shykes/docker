package ur

// This file defines a shim for a future "libsync"
// package with a standardized interface for synchronizing
// state between 2 remote peers.

type SyncReader interface {
	Next() ([]SyncChange, error)
}

type SyncWriter interface {
	Set(changes ...SyncChange) error
}

type SyncChange struct {
	Op    SyncOp
	Key   string
	Value string
}

type SyncOp int

const (
	OpSet SyncOp = iota
	OpDel
)
