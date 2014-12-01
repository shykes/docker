package libpack

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

var (
	// Scope values which should not actually change the scope
	nopScopes = []string{"", "/", "."}
)

func tmpdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "libpack-test-")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func tmpDB(t *testing.T, ref string) *DB {
	if ref == "" {
		ref = "refs/heads/test"
	}
	tmp := tmpdir(t)
	db, err := Init(tmp, ref)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func nukeDB(db *DB) {
	dir := db.Repo().Path()
	os.RemoveAll(dir)
}

func TestOpen(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := Init(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after init")
	}

	db2, err := Open(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}

	if db2 == nil {
		t.Fatal("db was nil after init")
	}

	_, err = Open("/nonexistentpath", "refs/heads/test")

	if err == nil {
		t.Fatal("Opening nonexistent path without forceInit did not yield an error")
	}
}

func TestOpenOrInit(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)

	db, err := OpenOrInit(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after ForceInit")
	}

	fi, err := os.Stat(path.Join(tmp, "refs"))
	if err != nil {
		t.Fatal("ForceInit did not create a repository")
	}

	db, err = OpenOrInit(tmp, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}

	if db == nil {
		t.Fatal("db was nil after ForceInit")
	}

	fi2, err := os.Stat(path.Join(tmp, "refs"))
	if err != nil {
		t.Fatal("ForceInit wiped out the repository!")
	}

	if fi.ModTime() != fi2.ModTime() {
		t.Fatal("ForceInit created a new repository and should have just opened it")
	}
}

// Pull on a non-empty destination (ref set and uncommitted changes are present)
func TestPullToUncommitted(t *testing.T) {
	db1 := tmpDB(t, "refs/heads/test1")
	defer nukeDB(db1)

	db2 := tmpDB(t, "")
	defer nukeDB(db2)

	db1.Set("foo/bar/baz", "hello world")
	db1.Mkdir("/etc/something")
	db1.Commit("just creating some stuff")

	db2.Set("uncommitted-key", "uncommitted value")
	if err := db2.Pull(db1.Repo().Path(), "refs/heads/test1"); err != nil {
		t.Fatal(err)
	}

	assertGet(t, db2, "foo/bar/baz", "hello world")
	assertNotExist(t, db2, "uncommitted-key")
}

func TestPush(t *testing.T) {
	src := tmpDB(t, "refs/heads/test")
	defer nukeDB(src)
	src.Set("foo/bar/baz", "hello world")
	src.Commit("")

	dst := tmpDB(t, "refs/heads/test")
	defer nukeDB(dst)
	dst.Set("committed-key", "this should go away")
	dst.Commit("")

	if err := src.Push(dst.Repo().Path(), "refs/heads/test"); err != nil {
		t.Fatal(err)
	}

	dst2, _ := Open(dst.Repo().Path(), "refs/heads/test")
	assertGet(t, dst2, "foo/bar/baz", "hello world")
	assertNotExist(t, dst2, "committed-key")
}

func TestInit(t *testing.T) {
	var err error
	// Init existing dir
	tmp1 := tmpdir(t)
	defer os.RemoveAll(tmp1)
	_, err = Init(tmp1, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	// Test that tmp1 is a bare git repo
	if _, err := os.Stat(path.Join(tmp1, "refs")); err != nil {
		t.Fatal(err)
	}

	// Init a non-existing dir
	tmp2 := path.Join(tmp1, "new")
	_, err = Init(tmp2, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	// Test that tmp2 is a bare git repo
	if _, err := os.Stat(path.Join(tmp2, "refs")); err != nil {
		t.Fatal(err)
	}

	// Init an already-initialized dir
	_, err = Init(tmp2, "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestScopeNoop(t *testing.T) {
	root := tmpDB(t, "")
	defer nukeDB(root)
	root.Set("foo/bar", "hello")
	for _, s := range nopScopes {
		scoped := root.Scope(s)
		assertGet(t, scoped, "foo/bar", "hello")
	}
}

func TestScopeDump(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	db.Set("a/b/c/foo", "bar")
	var buf bytes.Buffer
	db.Scope("a/b/c").Dump(&buf)
	if s := buf.String(); s != "foo = bar\n" {
		t.Fatalf("%#v", s)
	}
}

func TestScopeSetGet(t *testing.T) {
	root := tmpDB(t, "")
	defer nukeDB(root)
	scoped := root.Scope("foo/bar")
	scoped.Set("hello", "world")
	assertGet(t, scoped, "hello", "world")
	assertGet(t, root, "foo/bar/hello", "world")
}

func TestScopeTree(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	db.Set("a/b/c/d/hello", "world")
	tree, err := db.Scope("a/b/c/d").Tree()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	TreeDump(db.repo, tree, "/", &buf)
	if s := buf.String(); s != "hello = world\n" {
		t.Fatalf("%v", s)
	}
}

func TestMultiScope(t *testing.T) {
	root := tmpDB(t, "")
	defer nukeDB(root)
	root.Set("a/b/c/d", "hello")
	a := root.Scope("a")
	ab := a.Scope("b")
	var abDump bytes.Buffer
	ab.Dump(&abDump)
	if s := abDump.String(); s != "c/\nc/d = hello\n" {
		t.Fatalf("%v\n", s)
	}
}

// A convenience interface to allow querying DB and GlobalTree
// with the same utilities
type ReadDB interface {
	Get(string) (string, error)
	List(string) ([]string, error)
	Dump(io.Writer) error
}

func assertGet(t *testing.T, db ReadDB, key, val string) {
	if v, err := db.Get(key); err != nil {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		db.Dump(os.Stderr)
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert %v=%v db:%#v\n=> %#v", key, val, db, err)
	} else if v != val {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		db.Dump(os.Stderr)
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert %v=%v db:%#v\n=> %v=%#v", key, val, db, key, v)
	}
}

// Assert that the specified key does not exist in db
func assertNotExist(t *testing.T, db ReadDB, key string) {
	if _, err := db.Get(key); err == nil {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		db.Dump(os.Stderr)
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert key %v doesn't exist db:%#v\n=> %v", key, db, err)
	}
}

func TestSetEmpty(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo", ""); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	db.Set("foo", "bar")
	if db.tree == nil {
		t.Fatalf("%#v\n")
	}
	for _, rootpath := range []string{"", ".", "/", "////", "///."} {
		names, err := db.List(rootpath)
		if err != nil {
			t.Fatalf("%s: %v", rootpath, err)
		}
		if fmt.Sprintf("%v", names) != "[foo]" {
			t.Fatalf("List(%v) =  %#v", rootpath, names)
		}
	}
	for _, wrongpath := range []string{
		"does-not-exist",
		"sldhfsjkdfhkjsdfh",
		"a/b/c/d",
		"foo/sdfsdf",
	} {
		_, err := db.List(wrongpath)
		if err == nil {
			t.Fatalf("should fail: %s", wrongpath)
		}
		if !strings.Contains(err.Error(), "does not exist in the given tree") {
			t.Fatalf("wrong error: %v", err)
		}
	}
}

func TestSetGetSimple(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
}

func TestSetGetMultiple(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
	if key, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if key != "bu" {
		t.Fatalf("%#v", key)
	}
}

func TestCommitConcurrentNoConflict(t *testing.T) {
	db1 := tmpDB(t, "")
	defer nukeDB(db1)
	db2, _ := Open(db1.Repo().Path(), db1.ref)

	db1.Set("foo", "A")
	db2.Set("bar", "B")

	assertGet(t, db1, "foo", "A")
	assertGet(t, db2, "bar", "B")

	if err := db1.Commit("A"); err != nil {
		t.Fatal(err)
	}

	if err := db2.Commit("B"); err != nil {
		t.Fatalf("%#v", err)
	}

	db3, _ := Open(db1.Repo().Path(), db1.ref)
	assertGet(t, db3, "foo", "A")
	assertGet(t, db3, "bar", "B")
}

func TestCommitConcurrentWithConflict(t *testing.T) {
	db1 := tmpDB(t, "")
	defer nukeDB(db1)
	db2, _ := Open(db1.Repo().Path(), db1.ref)

	db1.Set("foo", "A")
	db2.Set("foo", "B")
	db1.Set("1", "written by 1")

	db1.Set("2", "written by 2")

	assertGet(t, db1, "foo", "A")
	assertGet(t, db2, "foo", "B")

	if err := db1.Commit("A"); err != nil {
		t.Fatal(err)
	}
	if err := db2.Commit("B"); err != nil {
		t.Fatalf("%#v", err)
	}

	db3, err := Open(db1.Repo().Path(), db1.ref)
	if err != nil {
		t.Fatal(err)
	}
	assertGet(t, db3, "foo", "B")
	assertGet(t, db3, "1", "written by 1")
	assertGet(t, db3, "2", "written by 2")
}

func TestSetCommitGet(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	if err := db.Set("ga", "added after commit"); err != nil {
		t.Fatal(err)
	}
	var err error
	db, err = Init(db.Repo().Path(), "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}
	if val, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if val != "bar" {
		t.Fatalf("%#v", val)
	}
	if val, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if val != "bu" {
		t.Fatalf("%#v", val)
	}
}

func TestSetGetNested(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("a/b/c/d/hello", "world"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("a/b/c/d/hello"); err != nil {
		t.Fatal(err)
	} else if key != "world" {
		t.Fatalf("%#v", key)
	}
}

func testSetGet(t *testing.T, refs []string, scopes []string, components ...[]string) {
	for _, ref := range refs {
		rootdb := tmpDB(t, ref)
		defer nukeDB(rootdb)
		for _, scope := range scopes {
			db := rootdb.Scope(scope)
			if len(components) == 0 {
				return
			}
			if len(components) == 1 {
				for _, k := range components[0] {
					if err := db.Set(k, "hello world"); err != nil {
						t.Fatal(err)
					}
				}
				for _, k := range components[0] {
					if v, err := db.Get(k); err != nil {
						t.Fatalf("Get('%s'): %v\n\troot=%#v\n\tscoped=%#v", k, err, rootdb, db)
					} else if v != "hello world" {
						t.Fatal(err)
					}
				}
				return
			}
			// len(components) >= 2
			first := make([]string, 0, len(components[0])*len(components[1]))
			for _, prefix := range components[0] {
				for _, suffix := range components[1] {
					first = append(first, path.Join(prefix, suffix))
				}
			}
			newComponents := append([][]string{first}, components[2:]...)
			testSetGet(t, []string{ref}, []string{scope}, newComponents...)
		}
	}
}

func TestSetGetNestedMultiple1(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"foo"}, []string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestSetGetNestedMultiple(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestSetGetNestedMultipleScoped(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{"0.1"},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestMkdir(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Mkdir("/"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if err := db.Mkdir("foo/bar"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckout(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp := tmpdir(t)
	defer os.RemoveAll(checkoutTmp)
	if _, err := db.Checkout(checkoutTmp); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path.Join(checkoutTmp, "foo/bar/baz"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("%#v", data)
	}
}

func TestCheckoutTmp(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp, err := db.Checkout("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(checkoutTmp)
	f, err := os.Open(path.Join(checkoutTmp, "foo/bar/baz"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("%#v", data)
	}
}

func TestCheckoutUncommitted(t *testing.T) {
	t.Skip("FIXME: DB.CheckoutUncommitted does not work properly at the moment")
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Set("foo/bar/baz", "hello world"); err != nil {
		t.Fatal(err)
	}
	if err := db.Commit("test"); err != nil {
		t.Fatal(err)
	}
	checkoutTmp := tmpdir(t)
	if err := db.CheckoutUncommitted(checkoutTmp); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path.Join(checkoutTmp, "foo/bar/baz"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("%#v", data)
	}
}

// Pull on an empty destination (ref not set)
func TestPullToEmpty(t *testing.T) {
	db1 := tmpDB(t, "refs/heads/test1")
	defer nukeDB(db1)

	db2 := tmpDB(t, "refs/heads/test-foo-bar")
	defer nukeDB(db2)

	db1.Set("foo/bar/baz", "hello world")
	db1.Mkdir("/etc/something")
	db1.Commit("just creating some stuff")

	if err := db2.Pull(db1.Repo().Path(), "refs/heads/test1"); err != nil {
		t.Fatal(err)
	}

	assertGet(t, db2, "foo/bar/baz", "hello world")
}

// Test Update when the ref has not changed
func TestUpdate(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	db.Set("key", "value")
	if err := db.Update(); err != nil {
		t.Fatal(err)
	}
	assertGet(t, db, "key", "value")

	if err := db.Commit(""); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(); err != nil {
		t.Fatal(err)
	}
	assertGet(t, db, "key", "value")
}

// Test Update when the ref has changed out of band
func TestUpdateWithChanges(t *testing.T) {
	db1 := tmpDB(t, "refs/heads/test")
	defer nukeDB(db1)
	db2, err := Open(db1.Repo().Path(), "refs/heads/test")
	if err != nil {
		t.Fatal(err)
	}

	db1.Set("key1", "val1")
	if err := db1.Commit("commit 1"); err != nil {
		t.Fatal(err)
	}

	db2.Set("something", "uncommitted change")
	if err := db2.Update(); err != nil {
		t.Fatal(err)
	}
	assertGet(t, db2, "key1", "val1")
	assertNotExist(t, db2, "something")

	db2.Set("key2", "val2")
	if err := db2.Commit("commit 2"); err != nil {
		t.Fatal(err)
	}

	if err := db1.Update(); err != nil {
		t.Fatal(err)
	}
	assertGet(t, db1, "key1", "val1")
	assertGet(t, db1, "key2", "val2")
}

func TestAddDB(t *testing.T) {
	db1 := tmpDB(t, "refs/heads/db1")
	defer nukeDB(db1)

	db2, err := Open(db1.Repo().Path(), "refs/heads/db2")
	if err != nil {
		t.Fatal(err)
	}
	defer nukeDB(db2)

	db1.Set("hello", "world")
	db1.Set("foo/bar/baz", "hello there")

	db2.Set("k", "v")
	db2.Set("db1/foo/bar/abc", "xyz")
	if err := db2.AddDB("db1", db1); err != nil {
		t.Fatal(err)
	}
	assertGet(t, db2, "db1/hello", "world")
	assertGet(t, db2, "k", "v")
	assertGet(t, db2, "db1/foo/bar/baz", "hello there")
	assertGet(t, db2, "db1/foo/bar/abc", "xyz")
	assertGet(t, db2, "db1/foo/bar/abc", "xyz")
}

func TestEmptyCommit(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)
	if err := db.Commit(""); err != nil {
		t.Fatal(err)
	}
	db.Set("foo", "bar")
	// This should commit something
	if err := db.Commit(""); err != nil {
		t.Fatal(err)
	}
	// This should commit nothing (but not fail)
	if err := db.Commit(""); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	db1 := tmpDB(t, "refs/heads/delete_test")
	defer nukeDB(db1)

	if err := db1.Set("test", "quux"); err != nil {
		t.Fatal(err)
	}

	str, err := db1.Get("test")
	if err != nil {
		t.Fatal(err)
	}

	if str != "quux" {
		t.Fatal("Test value was not retrieved with Get")
	}

	if err := db1.Commit(""); err != nil {
		t.Fatal(err)
	}

	if err := db1.Delete("test"); err != nil {
		t.Fatal(err)
	}

	if err := db1.Commit(""); err != nil {
		t.Fatal(err)
	}

	_, err = db1.Get("test")
	if err == nil {
		t.Fatal("Test key did not get deleted after delete/commit call")
	}
}
