package main

import (
	"bytes"
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	parser "github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/typesystem"
)

var (
	// TODO no global variable
	edgeCounter = 0

	// for computed usersets
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
		edgeCounter++
		options = append(options, graph.EdgeAttribute("label", strconv.Itoa(edgeCounter)))
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

func buildGraph(model *openfgav1.AuthorizationModel) (graph.Graph[string, string], error) {
	typesys := typesystem.New(model)

	// sort type names to guarantee stable outcome
	sort.SliceStable(model.GetTypeDefinitions(), func(i, j int) bool {
		return slices.IsSorted([]string{model.GetTypeDefinitions()[i].Type, model.GetTypeDefinitions()[j].Type})
	})

	g := graph.New(graph.StringHash, graph.Directed())

	for _, typedef := range model.GetTypeDefinitions() {
		typeName := typedef.GetType()

		if err := addNode(g, typeName); err != nil {
			return nil, err
		}

		if err := addNode(g, typeName+":*"); err != nil {
			return nil, err
		}

		// sort relation names to guarantee stable outcome
		sortedRelationNames := make([]string, 0, len(typedef.GetRelations()))
		for key := range typedef.GetRelations() {
			sortedRelationNames = append(sortedRelationNames, key)
		}
		sort.Strings(sortedRelationNames)

		for _, relation := range sortedRelationNames {
			rewrite := typedef.GetRelations()[relation]
			relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)
			if err := addNode(g, relationNodeName); err != nil {
				return nil, err
			}

			_, err := typesystem.WalkUsersetRewrite(rewrite, rewriteHandler(typesys, g, typeName, relation))
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
				conditionName := assignableRelation.GetCondition()
				if conditionName != "" {
					assignableType = fmt.Sprintf(" %s[with %s]", assignableType, conditionName)
				}

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
			rewrittenRelation := rw.TupleToUserset.GetComputedUserset().GetRelation()

			tuplesetRel, err := typesys.GetRelation(typeName, tupleset)
			if err != nil {
				return err
			}

			directlyRelatedTypes := tuplesetRel.GetTypeInfo().GetDirectlyRelatedUserTypes()
			for _, relatedType := range directlyRelatedTypes {
				assignableType := relatedType.GetType()
				conditionName := relatedType.GetCondition()
				if conditionName != "" {
					assignableType = fmt.Sprintf(" %s[with %s]", assignableType, conditionName)
				}
				rewrittenNodeName := fmt.Sprintf("%s#%s", assignableType, rewrittenRelation)
				conditionedOnNodeName := fmt.Sprintf("(%s#%s)", typeName, tuplesetRel.GetName())
				edgeLabelAttribute := graph.EdgeAttribute("headlabel", conditionedOnNodeName)

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

func Writer(modelString string) string {
	model := parser.MustTransformDSLToProto(modelString)

	g, err := buildGraph(model)
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

	var writer bytes.Buffer
	err = draw.DOT(g, &writer, draw.GraphAttribute("rankdir", "BT"))
	if err != nil {
		log.Fatalf("failed to render graph: %v", err)
	}

	return writer.String()
}
