package extensions

import (
	"errors"
	"io/ioutil"
	"os"
	"sort"
	"testing"
)

var expectingErr = errors.New("error expected")

func TestStateGoBackend(t *testing.T) {
	s, err := NewDummyGitState()
	if err != nil {
		t.Fatal(err)
	}

	s.List("/")
	s.Set("foo", "bar")
	s.Get("foo")
	s.List("/")
	s.Set("hello", "world")
	s.List("/")
	s.Set("sub/path/key", "value")
	s.List("/")
	s.List("/sub")
	s.List("/sub/path/")
	s.Mkdir("/dir")
	// order does not matter, since it is sorted in list()
	s.List("/")
	s.Remove("/sub")
	s.List("/")
	s.List("/sub/path")
	s.Get("/sub/path/key")

	s.Close()
}

func TestStateSingleHost(t *testing.T) {
	tmp, err := ioutil.TempDir("", "TestState")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	s, err := GitStateFromFolder(tmp, "test")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}

	expect := expecter{T: t, s: s}

	expect.list("/", []string{})
	expect.set("foo", "bar")
	expect.get("foo", "bar")
	expect.list("/", []string{"foo"})
	expect.set("hello", "world")
	expect.list("/", []string{"foo", "hello"})
	expect.set("sub/path/key", "value")
	expect.list("/", []string{"foo", "hello", "sub"})
	expect.list("/sub", []string{"path"})
	expect.list("/sub/path/", []string{"key"})
	expect.mkdir("/dir")
	// order does not matter, since it is sorted in list()
	expect.list("/", []string{"foo", "hello", "sub", "dir"})
	expect.remove("/sub")
	expect.list("/", []string{"foo", "hello", "dir"})
	expect.list("/sub/path", nil, expectingErr)
	expect.get("/sub/path/key", "", expectingErr)

	s.Close()

	// let's reopen the same state from disk
	s, err = GitStateFromFolder(tmp, "test")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}

	expect = expecter{T: t, s: s}
	// our keys did not disappear after closing and reopening
	expect.list("/", []string{"foo", "hello", "dir"})

	expect.remove("/dir")
	expect.remove("/foo")
	expect.remove("/hello")
	expect.list("/", []string{})
	s.Close()
}

type expecter struct {
	*testing.T
	s *GitState
}

func (t expecter) list(root string, expect []string, errs ...error) {
	list, err := t.s.List(root)
	if err != nil {
		if len(errs) > 0 && errs[0] != nil {
			// error is expected
			t.Logf("All good, error was expected: %v", err)
			return
		}
		t.Fatal(err)
	}
	if len(list) != len(expect) {
		t.Fatalf(`list=%v, expected %v`, list, expect)
	}
	sort.Strings(expect)
	for i, e := range list {
		if e != expect[i] {
			t.Fatalf(`list=%v, expected %v`, list, expect)
		}
	}
}

func (t expecter) get(key, expect string, errs ...error) {
	value, err := t.s.Get(key)
	if err != nil {
		if len(errs) > 0 && errs[0] != nil {
			// error is expected
			t.Logf("expected error: %v", err)
			return
		}
		t.Fatal(err)
	}
	if value != expect {
		t.Fatalf(`get(%q)=%q, expected %q`, key, value, expect)
	}
}

func (t expecter) set(key, value string) {
	if err := t.s.Set(key, value); err != nil {
		t.Fatal(err)
	}
}

func (t expecter) mkdir(key string) {
	if err := t.s.Mkdir(key); err != nil {
		t.Fatal(err)
	}
}

func (t expecter) remove(key string) {
	if err := t.s.Remove(key); err != nil {
		t.Fatal(err)
	}
}

func (t expecter) Fatalf(format string, args ...interface{}) {
	t.s.Close()
	t.T.Fatalf(format, args...)
}

func (t expecter) Fatal(args ...interface{}) {
	t.s.Close()
	t.T.Fatal(args...)
}
