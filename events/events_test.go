package events

import (
	"testing"
	"github.com/dotcloud/docker/engine"
)


func TestLogEvent(t *testing.T) {
	eng := engine.Tmp(t)
	defer eng.Nuke()
	if err := NewLogger().Install(eng); err != nil {
		t.Fatal(err)
	}
	
}
