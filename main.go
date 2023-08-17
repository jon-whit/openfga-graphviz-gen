package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	parser "github.com/craigpastro/openfga-dsl-parser/v2"
	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/openfga/pkg/typesystem"
)

var (
	styleAttrDashed = graph.EdgeAttribute("style", "dashed")
)

func addNode(g graph.Graph[string, string], v string) error {
	err := g.AddVertex(v)
	if err != nil && err != graph.ErrVertexAlreadyExists {
		log.Println(err)
		return err
	}

	return nil
}

func addEdge(g graph.Graph[string, string], from, to string, options ...func(*graph.EdgeProperties)) error {
	if _, err := g.Vertex(from); err != nil {
		return addNode(g, from)
	}

	if _, err := g.Vertex(to); err != nil {
		return addNode(g, to)
	}

	_, err := g.Edge(from, to)
	if err == graph.ErrEdgeNotFound {
		if err = g.AddEdge(from, to, options...); err != nil {
			log.Println(err)
			return err
		}
	}

	if err = g.UpdateEdge(from, to, options...); err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func buildGraph(typedefs []*openfgav1.TypeDefinition) (graph.Graph[string, string], error) {
	typesys, err := typesystem.NewAndValidate(context.Background(), &openfgav1.AuthorizationModel{
		SchemaVersion:   typesystem.SchemaVersion1_1,
		TypeDefinitions: typedefs,
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(typedefs, func(i, j int) bool {
		return typedefs[i].GetType() < typedefs[j].GetType()
	})

	g := graph.New(graph.StringHash, graph.Directed())

	for _, typedef := range typedefs {
		typeName := typedef.GetType()

		if err := addNode(g, typeName); err != nil {
			return nil, err
		}

		if err := addNode(g, typeName+":*"); err != nil {
			return nil, err
		}

		for relation, rewrite := range typedef.GetRelations() {
			relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)
			if err := addNode(g, relationNodeName); err != nil {
				return nil, err
			}

			_, err = typesystem.WalkUsersetRewrite(rewrite, rewriteHandler(typesys, g, typeName, relation))
			if err != nil {
				return nil, fmt.Errorf("failed to WalkUsersetRewrite tree: %v", err)
			}
		}
	}

	return g, nil
}

func rewriteHandler(typesys *typesystem.TypeSystem, g graph.Graph[string, string], typeName, relation string) typesystem.WalkUsersetRewriteHandler {
	relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)

	return func(r *openfgav1.Userset) interface{} {
		switch rw := r.Userset.(type) {
		case *openfgav1.Userset_This:
			assignableRelations, err := typesys.GetDirectlyRelatedUserTypes(typeName, relation)
			if err != nil {
				return err
			}

			for _, assignableRelation := range assignableRelations {
				assignableType := assignableRelation.GetType()

				if assignableRelation.GetRelationOrWildcard() != nil {
					assignableRelationRef := assignableRelation.GetRelation()
					if assignableRelationRef != "" {
						assignableRelationNodeName := fmt.Sprintf("%s#%s", assignableType, assignableRelationRef)

						if err := addEdge(g, assignableRelationNodeName, relationNodeName); err != nil {
							return err
						}
					}

					wildcardRelationRef := assignableRelation.GetWildcard()
					if wildcardRelationRef != nil {
						wildcardRelationNodeName := fmt.Sprintf("%s:*", assignableType)

						if err := addEdge(g, wildcardRelationNodeName, relationNodeName); err != nil {
							return err
						}
					}
				} else {
					if err := addEdge(g, assignableType, relationNodeName); err != nil {
						return err
					}
				}
			}

			return nil
		case *openfgav1.Userset_ComputedUserset:
			rewrittenRelation := rw.ComputedUserset.GetRelation()
			rewritten, err := typesys.GetRelation(typeName, rewrittenRelation)
			if err != nil {
				return err
			}

			rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewritten.GetName())
			if err := addEdge(g, rewrittenNodeName, relationNodeName); err != nil {
				return err
			}

			return nil
		case *openfgav1.Userset_TupleToUserset:
			tupleset := rw.TupleToUserset.GetTupleset().GetRelation()
			rewrittenRelation := rw.TupleToUserset.ComputedUserset.GetRelation()

			tuplesetRel, err := typesys.GetRelation(typeName, tupleset)
			if err != nil {
				return err
			}

			var edgeLabels []string
			directlyRelatedTypes := tuplesetRel.GetTypeInfo().GetDirectlyRelatedUserTypes()
			for _, relatedType := range directlyRelatedTypes {
				assignableType := relatedType.GetType()
				edgeLabels = append(edgeLabels, fmt.Sprintf("%s#%s", assignableType, tupleset))
			}

			if len(edgeLabels) > 0 {
				rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewrittenRelation)
				edgeLabelAttribute := graph.EdgeAttribute("label", strings.Join(edgeLabels, ", "))

				err := addEdge(g, rewrittenNodeName, relationNodeName, edgeLabelAttribute)
				if err != nil {
					return err
				}
			}

			return nil
		case *openfgav1.Userset_Union:
			return nil
		case *openfgav1.Userset_Intersection:
			// TODO: handle recursion
			children := rw.Intersection.GetChild()
			for _, child := range children {
				switch childrw := child.Userset.(type) {
				case *openfgav1.Userset_ComputedUserset:
					rewrittenRelation := childrw.ComputedUserset.GetRelation()
					rewritten, err := typesys.GetRelation(typeName, rewrittenRelation)
					if err != nil {
						return err
					}

					rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewritten.GetName())
					if err := addEdge(g, rewrittenNodeName, relationNodeName, styleAttrDashed); err != nil {
						return err
					}
				}
			}
			return nil
		case *openfgav1.Userset_Difference:
			return nil
		default:
			return fmt.Errorf("unexpected userset rewrite type encountered")
		}
	}
}

func main() {
	modelPathFlag := flag.String("model-path", "", "the file path for the OpenFGA model (in DSL format)")
	outputPathFlag := flag.String("output-path", "", "the file path for the output graph (default to stdout)")

	flag.Parse()

	bytes, err := os.ReadFile(*modelPathFlag)
	if err != nil {
		log.Fatalf("failed to read model file: %v", err)
	}

	typedefs := parser.MustParse(string(bytes))

	g, err := buildGraph(typedefs)
	if err != nil {
		log.Fatalf("failed to build graph: %v", err)
	}

	adjMap, err := g.AdjacencyMap()
	if err != nil {
		log.Fatalf("failed to compute adjacency map: %v", err)
	}

	// remove vertices with no edges
	for k, v := range adjMap {
		if len(v) == 0 {
			_ = g.RemoveVertex(k)
		}
	}

	var writer io.Writer
	if *outputPathFlag != "" && *outputPathFlag != "-" {
		writer, _ = os.Create(*outputPathFlag)
	} else {
		writer = os.Stdout
	}

	err = draw.DOT(g, writer, draw.GraphAttribute("rankdir", "BT"))
	if err != nil {
		log.Fatalf("failed to render graph: %v", err)
	}
}
