package journal

import (
	"database/sql"
	"strings"
	"log"
	"fmt"
	"os"
	_ "code.google.com/p/gosqlite/sqlite3"
)

type Action struct {
	Id		int64
	Actor		string
	Target		string
	Cmd		[]string
	Completed	bool
}


func Start(db, actor, target string, cmd ...string) (id int64, err error) {
	conn, err := sql.Open("sqlite3", db)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	if _, err := conn.Exec(`
	CREATE TABLE IF NOT EXISTS journal(
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target text,
		actor  text,
		cmd    text,
		completed	bool);
	`); err != nil {
		return -1, err
	}

	if _, err := conn.Exec("BEGIN EXCLUSIVE TRANSACTION"); err != nil {
		return -1, err
	}

	 r, err := conn.Exec(
		"INSERT INTO journal(actor, target, cmd, completed) VALUES(?, ?, ?, ?);",
		actor,
		target,
		strings.Join(cmd, " "),
		false,
	)
	if err != nil {
		return -1, err
	}
	id, err = r.LastInsertId()
	if err != nil {
		return -1, err
	}
	if _, err := conn.Exec("END TRANSACTION"); err != nil {
		return -1, err
	}
	return
}

func main() {
	if len(os.Args) > 1 {
		id, err := Start("test.db", "docker-42.42", "redis", os.Args[1:]...)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d\n", id)
	}
}
