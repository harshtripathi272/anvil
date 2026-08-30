package main

import (
	"fmt"

	"anvil"
)

func cmdCreate(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil create <db>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	fmt.Printf("created %s\n", args[0])
	return nil
}

func cmdPut(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: anvil put <db> <key> <value>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Put([]byte(args[1]), []byte(args[2])); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func cmdGet(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: anvil get <db> <key>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()
	v, err := db.Get([]byte(args[1]))
	if err == anvil.ErrNotFound {
		fmt.Println("NOT FOUND")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", v)
	return nil
}

func cmdDel(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: anvil del <db> <key>")
	}
	db, err := anvil.Open(args[0])
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Delete([]byte(args[1])); err != nil {
		return err
	}
	fmt.Println("OK")
	return nil
}

func cmdScan(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: anvil scan <db> [prefix]")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginRead()
	if err != nil {
		return err
	}
	defer tx.Close()

	var start, end []byte
	if len(args) == 2 && args[1] != "" {
		start = []byte(args[1])
		end = prefixEnd(start)
	}
	it := tx.Scan(start, end)
	n := 0
	for it.Next() {
		fmt.Printf("%s\t%s\n", it.Key(), it.Value())
		n++
	}
	if err := it.Err(); err != nil {
		return err
	}
	fmt.Printf("(%d keys)\n", n)
	return nil
}

func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil info <db>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()
	gen, root, pages := db.Info()
	fmt.Printf("ANVIL INFO\n\n")
	fmt.Printf("format version:   %d\n", anvil.FormatVersion)
	fmt.Printf("page size:        %d\n", anvil.PageSize)
	fmt.Printf("generation:       %d\n", gen)
	fmt.Printf("root page:        %d\n", root)
	fmt.Printf("pages allocated:  %d\n", pages)
	return nil
}

func cmdCheck(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: anvil check <db>")
	}
	db, err := anvil.Open(args[0], anvil.WithReadOnly())
	if err != nil {
		return err
	}
	defer db.Close()

	rep, err := db.Check()
	fmt.Printf("ANVIL CHECK\n\n")
	if err != nil {
		fmt.Printf("RESULT: FAIL\n  %v\n", err)
		return err
	}
	fmt.Printf("metadata:              OK\n")
	fmt.Printf("root:                  OK\n")
	fmt.Printf("pages checked:         %d\n", rep.PagesChecked)
	fmt.Printf("leaf depth:            %d\n", rep.LeafDepth)
	fmt.Printf("keys:                  %d\n", rep.Keys)
	fmt.Printf("checksum failures:     0\n")
	fmt.Printf("ordering violations:   0\n")
	fmt.Printf("leaf-depth violations: 0\n\n")
	fmt.Printf("RESULT: PASS\n")
	return nil
}

// prefixEnd returns the smallest key strictly greater than every key with the
// given prefix, or nil if the prefix is all 0xff (unbounded above).
func prefixEnd(prefix []byte) []byte {
	end := make([]byte, len(prefix))
	copy(end, prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}
