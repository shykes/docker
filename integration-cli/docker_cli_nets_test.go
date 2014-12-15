package main

import (
	"strings"
	"testing"
)

func deleteNetwork(t *testing.T, network string) {
	out, _, err := dockerCmd(t, "nets", "delete", network)
	if err != nil {
		t.Fatalf("deleting network %q failed with err: %v (output: %q)", network, err, out)
	}
}

func TestNetsDefault(t *testing.T) {
	out, _, err := dockerCmd(t, "nets")
	if err != nil {
		t.Fatal(err)
	}

	// FIXME Verify that a default network is part of the output
	t.Fatal("not implemented")

	logDone("nets - default network exists")
}

func TestNetsCreate(t *testing.T) {
	name := "new_network"
	out, _, err := dockerCmd(t, "nets", "create", name)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteNetwork(t, name)

	// Create duplicate network
	out, _, err = dockerCmd(t, "nets", "create", name)
	if err == nil {
		t.Fatalf("created a duplicate network: %q", out)
	}

	logDone("nets - create network")
}

func TestNetsDelete(t *testing.T) {
	// Delete non-existing network
	out, _, err := dockerCmd(t, "nets", "delete", "foo")
	if err == nil { // FIXME Verify error message
		t.Fatalf("deleted a non-existing network: %q", out)
	}
}

func TestNetsJoin(t *testing.T) {
	out, _, err := dockerCmd(t, "run", "-d", "-ti", "busybox")
	if err != nil {
		t.Fatal(err)
	}
	defer deleteAllContainers()
	containerID := strings.TrimSpace(out)

	// Try to join non-existing network
	out, _, err = dockerCmd(t, "nets", "join", containerID, "foo")
	if err == nil { // FIXME Verify error message
		t.Fatalf("joined a non-existing network: %q", out)
	}

	// Try to have a non-existing container join an existing network
	out, _, err = dockerCmd(t, "nets", "join", "foo", "default")
	if err == nil { // FIXME Verify error message
		t.Fatalf("joined with a non-existing container: %q", out)
	}

	// Standard use case
	out, _, err = dockerCmd(t, "nets", "join", containerID, "default")
	if err != nil {
		t.Fatal(err)
	}

	logDone("nets - join network")
}
