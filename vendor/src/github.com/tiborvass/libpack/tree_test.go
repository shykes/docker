package libpack

import (
	"os"
	"testing"

	git "github.com/libgit2/git2go"
)

func tmpRepo(t *testing.T) *git.Repository {
	tmp := tmpdir(t)
	repo, err := git.InitRepository(tmp, true)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func nukeRepo(repo *git.Repository) {
	repo.Free()
	os.RemoveAll(repo.Path())
}

func TestPipeline(t *testing.T) {
	r := tmpRepo(t)
	p := NewPipeline(r).Set("foo", "bar").Set("a/b/c", "hello").Mkdir("a/b/c/d")
	if p == nil {
		t.Fatalf("%#v", p)
	}
	out, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	e, err := out.EntryByPath("foo")
	if err != nil {
		t.Fatal(err)
	}
	val, err := lookupBlob(r, e.Id)
	if err != nil {
		t.Fatal(err)
	}
	if string(val.Contents()) != "bar" {
		t.Fatalf("%#v", out)
	}
	e, err = out.EntryByPath("a/b/c")
	if err != nil {
		t.Fatalf("%#v", e)
	}
	if e.Type != git.ObjectTree {
		t.Fatalf("%#v", e)
	}
}

func TestEmptyTree(t *testing.T) {
	repo := tmpRepo(t)
	defer nukeRepo(repo)
	empty, err := emptyTree(repo)
	if err != nil {
		t.Fatal(err)
	}
	if empty.String() != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("%v", empty)
	}
}

func TestUpdateTree1(t *testing.T) {
	repo := tmpRepo(t)
	defer nukeRepo(repo)
	hello, err := repo.CreateBlobFromBuffer([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	emptyId, _ := emptyTree(repo)
	empty, _ := lookupTree(repo, emptyId)
	t1, err := treeAdd(repo, empty, "foo", hello, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t1, "foo", "hello")

	t2, err := treeAdd(repo, t1, "subtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2, "foo", "hello")
	assertBlobInTree(t, repo, t2, "subtree/foo", "hello")

	t3, err := treeAdd(repo, empty, "subtree/subsubtree", t1.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t3, "subtree/subsubtree/foo", "hello")

	t4, err := treeAdd(repo, t2, "/", t3.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t4, "foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/foo", "hello")
	assertBlobInTree(t, repo, t4, "subtree/subsubtree/foo", "hello")

	t1b, err := treeAdd(repo, empty, "bar", hello, true)
	if err != nil {
		t.Fatal(err)
	}

	t2b, err := treeAdd(repo, t1, "/", t1b.Id(), true)
	if err != nil {
		t.Fatal(err)
	}
	assertBlobInTree(t, repo, t2b, "foo", "hello")
	assertBlobInTree(t, repo, t2b, "bar", "hello")
}

func assertBlobInTree(t *testing.T, repo *git.Repository, tree *git.Tree, key, value string) {
	e, err := tree.EntryByPath(key)
	if err != nil || e == nil {
		t.Fatalf("No blob at key %v.\n\ttree=%#v\n", key, tree)
	}
	blob, err := lookupBlob(repo, e.Id)
	if err != nil {
		t.Fatalf("No blob at key %v.\\n\terr=%v\n\ttree=%#v\n", key, err, tree)
	}
	if string(blob.Contents()) != value {
		t.Fatalf("blob at key %v != %v.\n\ttree=%#v\n\treal val = %v\n", key, value, tree, string(blob.Contents()))
	}
	blob.Free()
}

func assertBlobNotInTree(t *testing.T, repo *git.Repository, tree *git.Tree, key string) {
	_, err := tree.EntryByPath(key)
	if err == nil {
		t.Fatalf("Key %q still exists in tree", key)
	}
}
