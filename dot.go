package main

import (
	"strconv"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/encoding/dot"
	"gonum.org/v1/gonum/graph/multi"
)

type dotEncodingGraph struct {
	*multi.DirectedGraph
	edgeCounter    int
	mapping        map[string]int64            // node labels to node IDs
	reverseMapping map[int64]string            // node IDs to node labels
	lines          map[int64]map[string]string // line IDs to line attrs
}

var _ encoding.Attributer = (*dotEncodingGraph)(nil)

func (g *dotEncodingGraph) DOTAttributers() (graph, node, edge encoding.Attributer) {
	return g, nil, nil
}

func newDotEncodingGraph() *dotEncodingGraph {
	g := multi.NewDirectedGraph()
	return &dotEncodingGraph{g, 0, make(map[string]int64), make(map[int64]string), make(map[int64]map[string]string)}
}

var _ dot.Attributers = (*dotEncodingGraph)(nil)

func (g *dotEncodingGraph) Attributes() []encoding.Attribute {
	return []encoding.Attribute{{
		Key:   "rankdir",
		Value: "BT",
	}}
}

func (g *dotEncodingGraph) RemoveNodesWithNoEdges() {
	iter := g.Nodes()
	for {
		if !iter.Next() {
			break
		}
		n := iter.Node()
		if !g.DirectedGraph.From(n.ID()).Next() && !g.DirectedGraph.To(n.ID()).Next() {
			g.RemoveNode(n.ID())
		}
	}
}

func (g *dotEncodingGraph) NewNode() *dotNode {
	return &dotNode{Node: g.DirectedGraph.NewNode(), attrs: make(map[string]string)}
}

func (g *dotEncodingGraph) NewLine(from, to graph.Node) *dotLine {
	return &dotLine{Line: g.DirectedGraph.NewLine(from, to), attrs: make(map[string]string)}
}

func (g *dotEncodingGraph) AddOrGetNode(label string) graph.Node {
	if id, ok := g.mapping[label]; ok {
		return g.Node(id)
	}
	//fmt.Println("[AddOrGetNode] adding node", label)
	n := g.NewNode()
	g.DirectedGraph.AddNode(n)
	g.mapping[label] = n.ID()
	g.reverseMapping[n.ID()] = label
	n.attrs["label"] = label
	return n
}

func (g *dotEncodingGraph) AddEdge(from, to string, optionalHeadLabel string) graph.Line {
	n1 := g.AddOrGetNode(from)
	n2 := g.AddOrGetNode(to)
	existingLinesIter := g.Lines(n1.ID(), n2.ID())
	for {
		if !existingLinesIter.Next() {
			break
		}
		e := existingLinesIter.Line()
		if g.lines[e.ID()]["headlabel"] == optionalHeadLabel {
			// duplicate!
			return nil
		}
	}
	g.edgeCounter = g.edgeCounter + 1
	//fmt.Println("adding edge", from, "-->", to, "[", g.edgeCounter, "]", "headlabel", optionalHeadLabel)
	edge := g.NewLine(n1, n2)
	g.DirectedGraph.SetLine(edge)
	edge.attrs["label"] = strconv.Itoa(g.edgeCounter)
	edge.attrs["headlabel"] = optionalHeadLabel
	g.lines[edge.ID()] = edge.attrs
	return &dotLine{
		Line:  edge,
		attrs: edge.attrs,
	}
}

var _ encoding.Attributer = (*dotNode)(nil)

type dotNode struct {
	graph.Node
	attrs map[string]string
}

func (d *dotNode) Attributes() []encoding.Attribute {
	var attrs []encoding.Attribute

	for k, val := range d.attrs {
		attrs = append(attrs, encoding.Attribute{
			Key:   k,
			Value: val,
		})
	}
	return attrs
}

var _ encoding.Attributer = (*dotLine)(nil)

type dotLine struct {
	graph.Line
	attrs map[string]string
}

func (d *dotLine) Attributes() []encoding.Attribute {
	var attrs []encoding.Attribute

	for k, val := range d.attrs {
		attrs = append(attrs, encoding.Attribute{
			Key:   k,
			Value: val,
		})
	}
	return attrs
}
