package libpack

import (
	"testing"
)

const EmptyTreeId string = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func TestEmpty(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	p := NewPipeline(r)
	tree, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	if id := tree.Id().String(); id != EmptyTreeId {
		t.Fatalf("%v", id)
	}
}

func TestSet(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("foo", "bar").Set("a/b/c/d", "hello world").Set("foo", "baz").Run()
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, r, tree, "foo", "baz")
	assertBlobInTree(t, r, tree, "a/b/c/d", "hello world")
}

func TestAddTree(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree1, err := NewPipeline(r).Set("foo", "bar").Run()
	if err != nil {
		t.Fatal(err)
	}
	tree2, err := NewPipeline(r).Set("a/b/c/d", "hello world").Add("a", tree1, true).Run()
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, r, tree2, "a/b/c/d", "hello world")
	assertBlobInTree(t, r, tree2, "a/foo", "bar")
}

func TestAddPipeline(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	foobar := NewPipeline(r).Set("foo", "bar")
	tree, err := NewPipeline(r).Set("hello", "world").Set("foo", "abc").Add("subdir", foobar, true).Run()
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, r, tree, "hello", "world")
	assertBlobInTree(t, r, tree, "subdir/foo", "bar")
	assertBlobInTree(t, r, tree, "foo", "abc")
}

func TestDeletePipeline(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("hello", "world").Delete("hello").Run()
	if err != nil {
		t.Fatal(err)
	}
	assertBlobNotInTree(t, r, tree, "hello")
}

func TestScope(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("a/b/c/d", "hello").Scope("a/b/c").Run()
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, r, tree, "d", "hello")
}
