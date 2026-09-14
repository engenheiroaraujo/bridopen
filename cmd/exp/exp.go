package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/engenheiroaraujo/bridopen/views"
)

var (
	osExit  = os.Exit
	embedFS fs.FS = views.Files
	stdout  io.Writer = os.Stdout
	stderr  io.Writer = os.Stderr
)

func main() {
	if err := run(stdout, embedFS); err != nil {
		fmt.Fprintln(stderr, err)
		osExit(1)
	}
}

func run(out io.Writer, files fs.FS) error {
	data, err := fs.ReadFile(files, "static/js/core/index.js")
	if err != nil {
		fmt.Fprintln(out, err)
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
