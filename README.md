# openfga-graphviz-gen

Generate graphviz diagrams from an OpenFGA authorization model definition.

## Usage

To print the model: 

`make build && ./openfga-graphviz-gen --model-path <path>`

To generate a PNG of the model:

`make build && ./openfga-graphviz-gen --model-path <path> | dot -Tpng > model.png`