package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	parser "github.com/craigpastro/openfga-dsl-parser/v2"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"github.com/openfga/openfga/pkg/typesystem"
	openfgav1 "go.buf.build/openfga/go/openfga/api/openfga/v1"
)

func main() {
	modelPathFlag := flag.String("model-path", "", "the file path for the OpenFGA model (in DSL format)")
	targetObjectTypeFlag := flag.String("target-object-type", "", "the target object type")
	targetRelationFlag := flag.String("target-relation", "", "the relation on the target object type")
	sourceUserObjectTypeFlag := flag.String("source-user-object-type", "", "the source user object type")

	flag.Parse()

	bytes, err := os.ReadFile(*modelPathFlag)
	if err != nil {
		log.Fatalf("failed to read model file: %v", err)
	}

	typedefs := parser.MustParse(string(bytes))

	g := graphviz.New()
	graph, err := g.Graph(graphviz.Directed)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := graph.Close(); err != nil {
			log.Fatal(err)
		}

		g.Close()
	}()

	typesys, err := typesystem.NewAndValidate(context.Background(), &openfgav1.AuthorizationModel{
		SchemaVersion:   typesystem.SchemaVersion1_1,
		TypeDefinitions: typedefs,
	})
	if err != nil {
		log.Fatal(err)
	}

	nodes := map[string]*cgraph.Node{}

	for _, typedef := range typedefs {
		typeName := typedef.GetType()

		typeNode, err := graph.CreateNode(typeName)
		if err != nil {
			log.Fatal(err)
		}

		typeNode.SetShape(cgraph.SquareShape)

		if typeName == *targetObjectTypeFlag {
			typeNode.SetColor("magenta")
			typeNode.SetStyle(cgraph.FilledNodeStyle)
		}

		if typeName == *sourceUserObjectTypeFlag {
			typeNode.SetColor("cyan")
			typeNode.SetStyle(cgraph.FilledNodeStyle)
		}

		nodes[typeName] = typeNode

		for relation, _ := range typedef.GetRelations() {
			relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)
			relationNode, err := graph.CreateNode(relationNodeName)
			if err != nil {
				log.Fatal(err)
			}

			relationNode.SetShape(cgraph.CircleShape)
			relationNode.SetLabel(relation)

			if typeName == *targetObjectTypeFlag && relation == *targetRelationFlag {
				relationNode.SetColor("magenta")
				relationNode.SetStyle(cgraph.FilledNodeStyle)
			}

			nodes[relationNodeName] = relationNode
		}
	}

	for _, typedef := range typedefs {
		typeName := typedef.GetType()

		for relation, rewrite := range typedef.GetRelations() {
			relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)

			edgeLabel := fmt.Sprintf("%s#%s", typeName, relation)
			e, err := writeEdge(graph, nodes, typeName, relationNodeName, edgeLabel)
			if err != nil {
				log.Println(err)
			} else {
				e.SetDir(cgraph.BackDir)
			}

			assignableRelations, err := typesys.GetDirectlyRelatedUserTypes(typeName, relation)
			if err != nil {
				log.Fatal(err)
			}

			for _, assignableRelation := range assignableRelations {
				assignableType := assignableRelation.GetType()

				if assignableRelation.GetRelationOrWildcard() != nil {
					assignableRelationRef := assignableRelation.GetRelation()
					if assignableRelationRef != "" {
						edgeLabel := fmt.Sprintf("%s_%s_%s_%s", typeName, relation, assignableType, assignableRelationRef)

						assignableRelationNodeName := fmt.Sprintf("%s#%s", assignableType, assignableRelationRef)
						e, err := writeEdge(graph, nodes, relationNodeName, assignableRelationNodeName, edgeLabel)
						if err != nil {
							log.Println(err)
						} else {
							e.SetDir(cgraph.BackDir)
						}
					}
				} else {
					edgeLabel := fmt.Sprintf("%s_%s_%s", typeName, relation, assignableType)
					e, err := writeEdge(graph, nodes, relationNodeName, assignableType, edgeLabel)
					if err != nil {
						log.Println(err)
					} else {
						e.SetDir(cgraph.BackDir)
					}
				}
			}

			_, err = typesystem.WalkUsersetRewrite(rewrite, func(r *openfgav1.Userset) interface{} {
				switch rw := r.Userset.(type) {
				case *openfgav1.Userset_This:
					return nil
				case *openfgav1.Userset_ComputedUserset:
					rewrittenRelation := rw.ComputedUserset.GetRelation()
					rewritten, err := typesys.GetRelation(typeName, rewrittenRelation)
					if err != nil {
						return err
					}

					rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewritten.GetName())

					e, err := writeEdge(graph, nodes, relationNodeName, rewrittenNodeName, edgeLabel)
					if err != nil {
						log.Println(err)
					} else {
						e.SetDir(cgraph.BackDir)
						e.SetStyle(cgraph.DottedEdgeStyle)
					}

					return nil

				case *openfgav1.Userset_TupleToUserset:
					tupleset := rw.TupleToUserset.GetTupleset().GetRelation()
					rewrittenRelation := rw.TupleToUserset.ComputedUserset.GetRelation()

					tuplesetRel, err := typesys.GetRelation(typeName, tupleset)
					if err != nil {
						return err
					}

					directlyRelatedTypes := tuplesetRel.GetTypeInfo().GetDirectlyRelatedUserTypes()
					for _, relatedType := range directlyRelatedTypes {

						assignableType := relatedType.GetType()

						rewrittenRelationNodeName := fmt.Sprintf("%s#%s", assignableType, rewrittenRelation)

						edgeLabel := fmt.Sprintf("%s_%s_%s_%s", typeName, relation, assignableType, rewrittenRelation)
						e, err := writeEdge(graph, nodes, relationNodeName, rewrittenRelationNodeName, edgeLabel)
						if err != nil {
							log.Println(err)
						} else {
							e.SetDir(cgraph.BackDir)
							e.SetStyle(cgraph.DashedEdgeStyle)
						}
					}

					return nil
				case *openfgav1.Userset_Union:
					return nil
				case *openfgav1.Userset_Intersection:
					return nil
				case *openfgav1.Userset_Difference:
					return nil
				default:
					log.Fatalf("unexpected userset rewrite type encountered")
					return nil
				}
			})
			if err != nil {
				log.Fatalf("failed to WalkUsersetRewrite tree: %v", err)
			}
		}
	}

	if err := g.RenderFilename(graph, graphviz.PNG, "graph.png"); err != nil {
		log.Fatal(err)
	}
}

func writeEdge(graph *cgraph.Graph, nodes map[string]*cgraph.Node, from string, to string, label string) (*cgraph.Edge, error) {
	if _, ok := nodes[from]; !ok {
		return nil, fmt.Errorf("not found: %v", from)
	}
	if _, ok := nodes[to]; !ok {
		return nil, fmt.Errorf("not found: %v", to)
	}
	return graph.CreateEdge(label, nodes[from], nodes[to])
}
