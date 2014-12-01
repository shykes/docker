package libpack

import (
	"fmt"
	"io"
	"os"
	"path"

	git "github.com/libgit2/git2go"
)

// Removes a key from the tree.
func treeDel(repo *git.Repository, tree *git.Tree, key string) (*git.Tree, error) {
	if tree == nil {
		return nil, nil
	}

	var err error

	key = TreePath(key)
	base, leaf := path.Split(key)

	if tree != nil {
		if tree, err = TreeScope(repo, tree, base); err != nil {
			return nil, err
		}
	}

	builder, err := repo.TreeBuilderFromTree(tree)
	if err != nil {
		return nil, err
	}

	if err := builder.Remove(leaf); err != nil {
		return nil, err
	}

	treeId, err := builder.Write()
	if err != nil {
		return nil, err
	}

	newTree, err := lookupTree(repo, treeId)
	if err != nil {
		return nil, err
	}

	return newTree, err
}

// treeAdd creates a new Git tree by adding a new object
// to it at the specified path.
// Intermediary subtrees are created as needed.
// If an object already exists at key or any intermediary path,
// it is overwritten.
//
// - If merge is true, new trees are merged into existing ones at the
// file granularity (similar to 'cp -R').
// - If it is false, existing trees are completely shadowed (similar to 'mount')
//
// Since git trees are immutable, base is not modified. The new
// tree is returned.
// If an error is encountered, intermediary objects may be left
// behind in the git repository. It is the caller's responsibility
// to perform garbage collection, if any.
// FIXME: manage garbage collection, or provide a list of created
// objects.
func treeAdd(repo *git.Repository, tree *git.Tree, key string, valueId *git.Oid, merge bool) (t *git.Tree, err error) {
	/*
	** // Primitive but convenient tracing for debugging recursive calls to treeAdd.
	** // Uncomment this block for debug output.
	**
	** var callString string
	** if tree != nil {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree.Id(), key, valueId)
	** 	} else {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree, key, valueId)
	** 	}
	** 	fmt.Printf("   %s\n", callString)
	** 	defer func() {
	** 		if t != nil {
	** 			fmt.Printf("-> %s => %v\n", callString, t.Id())
	** 		} else {
	** 			fmt.Printf("-> %s => %v\n", callString, err)
	** 		}
	** 	}()
	 */
	if valueId == nil {
		return tree, nil
	}
	key = TreePath(key)
	base, leaf := path.Split(key)
	o, err := repo.Lookup(valueId)
	if err != nil {
		return nil, err
	}
	var builder *git.TreeBuilder
	if tree == nil {
		builder, err = repo.TreeBuilder()
		if err != nil {
			return nil, err
		}
	} else {
		builder, err = repo.TreeBuilderFromTree(tree)
		if err != nil {
			return nil, err
		}
	}
	defer builder.Free()
	// The specified path has only 1 component (the "leaf")
	if base == "" || base == "/" {
		// If val is a string, set it and we're done.
		// Any old value is overwritten.
		if _, isBlob := o.(*git.Blob); isBlob {
			if err := builder.Insert(leaf, valueId, 0100644); err != nil {
				return nil, err
			}
			newTreeId, err := builder.Write()
			if err != nil {
				return nil, err
			}
			newTree, err := lookupTree(repo, newTreeId)
			if err != nil {
				return nil, err
			}
			return newTree, nil
		}
		// If val is not a string, it must be a subtree.
		// Return an error if it's any other type than Tree.
		oTree, ok := o.(*git.Tree)
		if !ok {
			return nil, fmt.Errorf("value must be a blob or subtree")
		}
		var subTree *git.Tree
		var oldSubTree *git.Tree
		if tree != nil {
			oldSubTree, err = TreeScope(repo, tree, leaf)
			// FIXME: distinguish "no such key" error (which
			// FIXME: distinguish a non-existing previous tree (continue with oldTree==nil)
			// from other errors (abort and return an error)
			if err == nil {
				defer oldSubTree.Free()
			}
		}
		// If that subtree already exists, merge the new one in.
		if merge && oldSubTree != nil {
			subTree = oldSubTree
			for i := uint64(0); i < oTree.EntryCount(); i++ {
				var err error
				e := oTree.EntryByIndex(i)
				subTree, err = treeAdd(repo, subTree, e.Name, e.Id, merge)
				if err != nil {
					return nil, err
				}
			}
		} else {
			subTree = oTree
		}
		// If the key is /, we're replacing the current tree
		if key == "/" {
			return subTree, nil
		}
		// Otherwise we're inserting into the current tree
		if err := builder.Insert(leaf, subTree.Id(), 040000); err != nil {
			return nil, err
		}
		newTreeId, err := builder.Write()
		if err != nil {
			return nil, err
		}
		newTree, err := lookupTree(repo, newTreeId)
		if err != nil {
			return nil, err
		}
		return newTree, nil
	}
	subtree, err := treeAdd(repo, nil, leaf, valueId, merge)
	if err != nil {
		return nil, err
	}
	return treeAdd(repo, tree, base, subtree.Id(), merge)
}

func TreeGet(r *git.Repository, t *git.Tree, key string) (string, error) {
	if t == nil {
		return "", os.ErrNotExist
	}
	key = TreePath(key)
	e, err := t.EntryByPath(key)
	if err != nil {
		return "", err
	}
	blob, err := lookupBlob(r, e.Id)
	if err != nil {
		return "", err
	}
	defer blob.Free()
	return string(blob.Contents()), nil

}

func TreeList(r *git.Repository, t *git.Tree, key string) ([]string, error) {
	if t == nil {
		return []string{}, nil
	}
	subtree, err := TreeScope(r, t, key)
	if err != nil {
		return nil, err
	}
	defer subtree.Free()
	var (
		i     uint64
		count uint64 = subtree.EntryCount()
	)
	entries := make([]string, 0, count)
	for i = 0; i < count; i++ {
		entries = append(entries, subtree.EntryByIndex(i).Name)
	}
	return entries, nil
}

func TreeWalk(r *git.Repository, t *git.Tree, key string, h func(string, git.Object) error) error {
	if t == nil {
		return fmt.Errorf("no tree to walk")
	}
	subtree, err := TreeScope(r, t, key)
	if err != nil {
		return err
	}
	var handlerErr error
	err = subtree.Walk(func(parent string, e *git.TreeEntry) int {
		obj, err := r.Lookup(e.Id)
		if err != nil {
			handlerErr = err
			return -1
		}
		if err := h(path.Join(parent, e.Name), obj); err != nil {
			handlerErr = err
			return -1
		}
		obj.Free()
		return 0
	})
	if handlerErr != nil {
		return handlerErr
	}
	if err != nil {
		return err
	}
	return nil
}

func TreeDump(r *git.Repository, t *git.Tree, key string, dst io.Writer) error {
	return TreeWalk(r, t, key, func(key string, obj git.Object) error {
		if _, isTree := obj.(*git.Tree); isTree {
			fmt.Fprintf(dst, "%s/\n", key)
		} else if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(dst, "%s = %s\n", key, blob.Contents())
		}
		return nil
	})
}

func TreeScope(repo *git.Repository, tree *git.Tree, name string) (*git.Tree, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree undefined")
	}
	name = TreePath(name)
	if name == "/" {
		// Allocate a new Tree object so that the caller
		// can always call Free() on the result
		return lookupTree(repo, tree.Id())
	}
	entry, err := tree.EntryByPath(name)
	if err != nil {
		return nil, err
	}
	return lookupTree(repo, entry.Id)
}
