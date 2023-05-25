# openfga-graphviz-gen
Generate graphviz diagrams from an OpenFGA authorization model definition.

## Usage
`go run main.go --model-path <path>`

## Limitations
* The tool only accepts the DSL sytax that is recognized by [Craig Pastro's parser](https://github.com/craigpastro/openfga-dsl-parser).