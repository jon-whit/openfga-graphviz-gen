package main

import (
	"flag"
	"io"
	"log"
	"os"
)

func main() {
	modelPathFlag := flag.String("model-path", "", "the file path for the OpenFGA model (in DSL format)")
	outputPathFlag := flag.String("output-path", "", "the file path for the output graph (default to stdout)")

	flag.Parse()

	bytes, err := os.ReadFile(*modelPathFlag)
	if err != nil {
		log.Fatalf("failed to read model file: %v", err)
	}

	result, _ := Writer(string(bytes))

	var writer io.Writer
	if *outputPathFlag != "" && *outputPathFlag != "-" {
		writer, _ = os.Create(*outputPathFlag)
	} else {
		writer = os.Stdout
	}

	_, err = writer.Write([]byte(result))
	if err != nil {
		log.Fatalf("failed to render graph: %v", err)
	}
}
