package data

import (
	"testing"
	"strings"
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

func TestMsgReadFrom(t *testing.T) {
	m := make(Msg)
	_, err := m.ReadFrom(strings.NewReader("id=3\nparent-id=42\n"))
	if err != nil {
		t.Fatal(err)
	}
	if exp := m.Get("id"); exp != "3" {
		t.Fatalf("Unexpected value: %v", exp)
	}
	if exp := m.Get("parent-id"); exp != "42" {
		t.Fatalf("Unexpected value: %v", exp)
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
