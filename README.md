# openfga-graphviz-gen

Generate graphviz diagrams from an OpenFGA authorization model definition.

## Usage

To print the model: 

`go run main.go --model-path <path>`

To generate a PNG of the model:

`go run main.go --model-path <path> | dot -Tpng > model.png`