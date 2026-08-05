package main

import (
	"fmt"
	"io/fs"

	"github.com/anesthesia-fanruoxi/authplatform/web"
)

func main() {
	fs.WalkDir(web.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	})
}
