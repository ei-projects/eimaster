package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"

	"github.com/ei-projects/eimaster/pkg/lzevil"
)

func main() {
	decompress := flag.Bool("d", false, "decompress")
	flag.Parse()

	var reader io.Reader = os.Stdin
	var writer io.Writer = os.Stdout
	var inputSize int64 // For pack

	if *decompress {
		reader = lzevil.NewReader(reader)
	} else {
		var buf bytes.Buffer
		buf.ReadFrom(reader)
		reader = &buf
		inputSize = int64(buf.Len())
		writer = lzevil.NewWriter(writer, int(inputSize))
	}

	if _, err := io.Copy(writer, reader); err != nil && !errors.Is(err, io.EOF) {
		println("Unexpected error: " + err.Error())
		os.Exit(1)
	}
}
