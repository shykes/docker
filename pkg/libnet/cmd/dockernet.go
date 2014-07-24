package main

import (

)

func main() {
	// Usage examples:
	//
	// dockernet list [<scope>]
	// dockernet connect <scope>/<name> <target>
	// dockernet connect myapp/db mydb
	// dockernet disconnect myapp/db
	// dockernet sync
	// dockernet watch <scope>
	// dockernet query <scope> <filter>
	// dockernet open <scope> [<portspec>...]
	// dockernet close <scope> [<portspec>...]

	j, err := InitJournal(".git", "dockernet/0.0.1", "/")
	if err != nil {
		Fatalf("%v", err)
	}
	// ...
}


type Journal struct {
	repo *git.Repo
	branch string	// The branch name
	subtree string	// Under which subtree to scope this journal (for multi-tenancy)
}

func (j *Journal) Current() (*Tree, error) {

}

func (j *Journal) Get(hash string) (*Tree, error) {

}

func (j *Journal) Commit(desc []string, t *Tree) (string, error) {

}


