package libpack

import (
	"fmt"
	"os/exec"
	"strings"

	git "github.com/libgit2/git2go"
)

// A Pipeline defines a sequence of operations which can be run
// to produce a Git Tree.
// Pipelines make it easy to assemble trees of arbitrary complexity
// with relatively few lines of code.
// For example:
//
//   p := NewPipeline().Set("foo", "bar").Mkdir("/directory")
//   tree, _ := p.Run()
//
// Pipelines can be nested with Add:
//
//   p1 := NewPipeline().Set("hello", "world")
//   p2 := NewPipeline().Add("subdir", p1, true)
//   combotree, _ := p2.Run()
//
type Pipeline struct {
	repo *git.Repository
	prev *Pipeline
	op   TreeOp
	arg  interface{}
}

type addArg struct {
	key   string
	val   interface{}
	merge bool
}

// A TreeOp defines an individual operation operation in a pipeline.
type TreeOp int

const (
	OpEmpty TreeOp = iota
	OpBase
	OpSet
	OpMkdir
	OpAdd
	OpScope
	OpDelete
)

// NewPIpeline creates a new empty pipeline.
// Calling Run returns an empty tree.
func NewPipeline(repo *git.Repository) *Pipeline {
	return &Pipeline{
		repo: repo,
		op:   OpEmpty,
	}
}

// Set appends a new `set` instruction to a pipeline, and
// returns the new combined pipeline.
// `set` writes `value` in a blob at path `key` in input trees.
func (t *Pipeline) Set(key, value string) *Pipeline {
	return t.setPrev(OpSet, []string{key, value})
}

// Add appends a new `add` instruction to a pipeline, and
// returns the new combined pipeline.
// `add` inserts a git object in the input tree, at the pat 'key'.
// The following types are supported for `val`:
//  - git.Object: the specified object is added
//  - *git.Oid: the object at the specified ID is added
//  - *Pipeline: the specified pipeline is run, and the result is added
func (t *Pipeline) Add(key string, val interface{}, merge bool) *Pipeline {
	return t.setPrev(OpAdd, &addArg{
		key:   key,
		val:   val,
		merge: merge,
	})
}

// Delete appends a new `delete` instruction to a pipeline, then returns the
// combined Pipeline.
func (t *Pipeline) Delete(key string) *Pipeline {
	return t.setPrev(OpDelete, key)
}

// Mkdir appends a new `mkdir` instruction to a pipeline, and
// returns the new combined pipeline.
// `mkdir` inserts an empty subtree in the input tree, at
// the path `key`.
func (t *Pipeline) Mkdir(key string) *Pipeline {
	return t.setPrev(OpMkdir, key)
}

// Mkdir appends a new `base` instruction to a pipeline, and
// returns the new combined pipeline.
// `base` discards the input tree and outputs `base` instead.
func (t *Pipeline) Base(base *git.Tree) *Pipeline {
	return t.setPrev(OpBase, base)
}

func (t *Pipeline) Scope(key string) *Pipeline {
	return t.setPrev(OpScope, key)
}

// Run runs each step of the pipeline in sequence, each time passing
// the output of step N as input to step N+1.
// If an error is encountered, the pipeline is aborted.
func (t *Pipeline) Run() (*git.Tree, error) {
	var in *git.Tree
	// Call the previous operation before our own
	// (unless the current operation is Empty or Base, since they would
	// discard the result anyway)
	if t.prev != nil && t.op != OpEmpty && t.op != OpBase {
		prevOut, err := t.prev.Run()
		if err != nil {
			return nil, err
		}
		in = prevOut
	}
	switch t.op {
	case OpEmpty:
		{
			empty, err := emptyTree(t.repo)
			if err != nil {
				return nil, err
			}
			return lookupTree(t.repo, empty)
		}
	case OpBase:
		{
			base, ok := t.arg.(*git.Tree)
			if !ok {
				return nil, fmt.Errorf("base: invalid argument: %v", t.arg)
			}
			return base, nil
		}
	case OpAdd:
		{
			arg, ok := t.arg.(*addArg)
			if !ok {
				return nil, fmt.Errorf("add: invalid argument: %v", t.arg)
			}
			var id *git.Oid
			switch val := arg.val.(type) {
			case *Pipeline:
				{
					if out, err := val.Run(); err != nil {
						return nil, fmt.Errorf("add: run source: %v", err)
					} else {
						id = out.Id()
					}
				}
			case git.Object:
				{
					id = val.Id()
				}
			case *git.Oid:
				{
					id = val
				}
			case *DB:
				{
					tree, err := val.Tree()
					// FIXME: distinguish "no such entry" errors (which we can ignore)
					// from other errors (which we should forward)
					if err == nil && tree != nil {
						id = tree.Id()
					}
				}
			default:
				{
					return nil, fmt.Errorf("invalid value: %#v", val)
				}
			}
			return treeAdd(t.repo, in, arg.key, id, arg.merge)
		}
	case OpDelete:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("delete: invalid argument: %v", t.arg)
			}

			return treeDel(t.repo, in, key)
		}
	case OpMkdir:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("mkdir: invalid argument: %v", t.arg)
			}
			empty, err := emptyTree(t.repo)
			if err != nil {
				return nil, err
			}
			return treeAdd(t.repo, in, key, empty, true)
		}
	case OpSet:
		{
			kv, ok := t.arg.([]string)
			if !ok {
				return nil, fmt.Errorf("invalid argument")
			}
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid argument")
			}
			// FIXME: libgit2 crashes if value is empty.
			// Work around this by shelling out to git.
			var (
				id  *git.Oid
				err error
			)
			if kv[1] == "" {
				out, err := exec.Command("git", "--git-dir", t.repo.Path(), "hash-object", "-w", "--stdin").Output()
				if err != nil {
					return nil, fmt.Errorf("git hash-object: %v", err)
				}
				id, err = git.NewOid(strings.Trim(string(out), " \t\r\n"))
				if err != nil {
					return nil, fmt.Errorf("git newoid %v", err)
				}
			} else {
				id, err = t.repo.CreateBlobFromBuffer([]byte(kv[1]))
				if err != nil {
					return nil, err
				}
			}
			return treeAdd(t.repo, in, kv[0], id, true)
		}
	case OpScope:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("invalid argument")
			}
			return TreeScope(t.repo, in, key)
		}
	}
	return nil, fmt.Errorf("invalid op: %v", t.op)
}

func (t *Pipeline) setPrev(op TreeOp, arg interface{}) *Pipeline {
	return &Pipeline{
		prev: t,
		op:   op,
		arg:  arg,
		repo: t.repo,
	}
}
