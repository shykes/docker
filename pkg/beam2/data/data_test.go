package data

import (
	"testing"
)

func TestMsgAddGet(t *testing.T) {
	m := make(Msg)
	m.Add("foo", "bar")
	if v := m.Get("foo"); v != "bar" {
		t.Fatalf("Unexpected value: %v", v)
	}
}

func TestMsgSetGet(t *testing.T) {
	m := make(Msg)
	m.Set("foo", "bar")
	if v := m.Get("foo"); v != "bar" {
		t.Fatalf("Unexpected value: %v", v)
	}
}

func TestMsgGetNonexistent(t *testing.T) {
	m := make(Msg)
	m.Set("foo", "bar")
	if v := m.Get("somethingelse"); v != "" {
		t.Fatalf("Unexpected value: %v", v)
	}
}

func TestMsgArrays(t *testing.T) {
	m := make(Msg)
	m.Add("foo", "ga")
	m.Add("foo", "bu")
	m.Add("foo", "zo")
	m.Add("foo", "meu")
	values, exist := m["foo"]
	if !exist {
		t.Fatalf("Value should exist")
	}
	if len(values) != 4 {
		t.Fatalf("Unexpected value: %v", values)
	}
	if values[0] != "ga" || values[3] != "meu" {
		t.Fatalf("Unexpected value: %v", values)
	}
}
