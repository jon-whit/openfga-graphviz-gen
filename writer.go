package main

import (
	"fmt"
	"log"
	"slices"
	"sort"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	parser "github.com/openfga/language/pkg/go/transformer"
	"github.com/openfga/openfga/pkg/typesystem"
	"gonum.org/v1/gonum/graph/encoding/dot"
)

func buildGraph(model *openfgav1.AuthorizationModel) *dotEncodingGraph {
	typesys := typesystem.New(model)

	// sort type names to guarantee stable outcome
	sort.SliceStable(model.GetTypeDefinitions(), func(i, j int) bool {
		return slices.IsSorted([]string{model.GetTypeDefinitions()[i].Type, model.GetTypeDefinitions()[j].Type})
	})

	g := newDotEncodingGraph()

	for _, typedef := range model.GetTypeDefinitions() {
		typeName := typedef.GetType()

		g.AddOrGetNode(typeName)
		g.AddOrGetNode(typeName + ":*")

		// sort relation names to guarantee stable outcome
		sortedRelationNames := make([]string, 0, len(typedef.GetRelations()))
		for key := range typedef.GetRelations() {
			sortedRelationNames = append(sortedRelationNames, key)
		}
		sort.Strings(sortedRelationNames)

		for _, relation := range sortedRelationNames {
			g.AddOrGetNode(fmt.Sprintf("%s#%s", typeName, relation))

			rewrite := typedef.GetRelations()[relation]
			if _, err := typesystem.WalkUsersetRewrite(rewrite, rewriteHandler(typesys, g, typeName, relation)); err != nil {
				panic(err)
			}
		}
	}

	return g
}

func rewriteHandler(typesys *typesystem.TypeSystem, g *dotEncodingGraph, typeName, relation string) typesystem.WalkUsersetRewriteHandler {
	relationNodeName := fmt.Sprintf("%s#%s", typeName, relation)

	return func(r *openfgav1.Userset) interface{} {
		switch rw := r.Userset.(type) {
		case *openfgav1.Userset_This:
			assignableRelations, err := typesys.GetDirectlyRelatedUserTypes(typeName, relation)
			if err != nil {
				panic(err)
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

						g.AddEdge(assignableRelationNodeName, relationNodeName, "")
					}

					wildcardRelationRef := assignableRelation.GetWildcard()
					if wildcardRelationRef != nil {
						wildcardRelationNodeName := fmt.Sprintf("%s:*", assignableType)

						g.AddEdge(wildcardRelationNodeName, relationNodeName, "")
					}
				} else {
					g.AddEdge(assignableType, relationNodeName, "")
				}
			}

			return nil
		case *openfgav1.Userset_ComputedUserset:
			rewrittenRelation := rw.ComputedUserset.GetRelation()
			rewritten, err := typesys.GetRelation(typeName, rewrittenRelation)
			if err != nil {
				panic(err)
			}

			rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewritten.GetName())
			g.AddEdge(rewrittenNodeName, relationNodeName, "")

			return nil
		case *openfgav1.Userset_TupleToUserset:
			tupleset := rw.TupleToUserset.GetTupleset().GetRelation()
			rewrittenRelation := rw.TupleToUserset.GetComputedUserset().GetRelation()

			tuplesetRel, err := typesys.GetRelation(typeName, tupleset)
			if err != nil {
				panic(err)
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

				g.AddEdge(rewrittenNodeName, relationNodeName, conditionedOnNodeName)
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
						panic(err)
					}

					rewrittenNodeName := fmt.Sprintf("%s#%s", typeName, rewritten.GetName())
					g.AddEdge(rewrittenNodeName, relationNodeName, "")
				}
			}
			return nil
		case *openfgav1.Userset_Difference:
			return nil
		default:
			panic("unexpected userset rewrite type encountered")
		}
	}
}

func Writer(modelString string) string {
	model := parser.MustTransformDSLToProto(modelString)

	g := buildGraph(model)

	g.RemoveNodesWithNoEdges()

	multi, err := dot.MarshalMulti(g, "", "", "")
	if err != nil {
		log.Fatalf("failed to render graph: %v", err)
	}

	return string(multi)
}
