package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	testCases := map[string]struct {
		inputModel     string
		expectedOutput string
	}{
		`with_union`: { // https://github.com/openfga/openfga/blob/main/docs/list_objects/example/example.md
			inputModel: `
				model
					schema 1.1
				type user
				
				type group
				  relations
					define member: [user, group#member]
				
				type folder
				  relations
					define viewer: [user]
				
				type document
				  relations
					define parent: [folder]
					define editor: [user]
					define viewer: [user, user:*, group#member] or editor or viewer from parent`,
			expectedOutput: `digraph {
graph [
rankdir=BT
];

// Node definitions.
2 [label="document#editor"];
3 [label=user];
4 [label="document#parent"];
5 [label=folder];
6 [label="document#viewer"];
7 [label="user:*"];
8 [label="group#member"];
9 [label="folder#viewer"];

// Edge definitions.
2 -> 6 [
label=6
headlabel=""
];
3 -> 2 [
label=1
headlabel=""
];
3 -> 6 [
label=3
headlabel=""
];
3 -> 8 [
label=9
headlabel=""
];
3 -> 9 [
label=8
headlabel=""
];
5 -> 4 [
label=2
headlabel=""
];
7 -> 6 [
label=4
headlabel=""
];
8 -> 6 [
label=5
headlabel=""
];
8 -> 8 [
label=10
headlabel=""
];
9 -> 6 [
label=7
headlabel="(document#parent)"
];
}`,
		},
		`with_intersection`: { // https://github.com/openfga/openfga/blob/main/docs/list_objects/example_with_intersection_or_exclusion/example.md
			inputModel: `
				model
					schema 1.1
				type user
				type document
				   relations
					 define a: [user]
					 define b: [user]
					 define c: a and b`,
			expectedOutput: `digraph {
graph [
rankdir=BT
];

// Node definitions.
2 [label="document#a"];
3 [label=user];
4 [label="document#b"];
5 [label="document#c"];

// Edge definitions.
2 -> 5 [
label=3
headlabel=""
];
3 -> 2 [
label=1
headlabel=""
];
3 -> 4 [
label=2
headlabel=""
];
4 -> 5 [
label=4
headlabel=""
];
}`,
		},
		`with_exclusion`: { // https://github.com/openfga/openfga/blob/main/docs/list_objects/example_with_intersection_or_exclusion/example.md
			inputModel: `
				model
					schema 1.1
				type user
				type document
				   relations
					 define a: [user]
					 define b: [user]
					 define c: a but not b`,
			expectedOutput: `digraph {
graph [
rankdir=BT
];

// Node definitions.
2 [label="document#a"];
3 [label=user];
4 [label="document#b"];
5 [label="document#c"];

// Edge definitions.
2 -> 5 [
headlabel=""
label=3
];
3 -> 2 [
label=1
headlabel=""
];
3 -> 4 [
label=2
headlabel=""
];
4 -> 5 [
label=4
headlabel=""
];
}`,
		},
		`with_conditions`: {
			inputModel: `
			model
				schema 1.1
			
			type user
			
			type document
				relations
					define admin: [user with condition1]
					define writer: [user with condition2]
					define viewer: [user:* with condition3]
			
			condition condition1(x: int) {
				x < 100
			}
			
			condition condition2(x: int) {
				x < 100
			}
			
			condition condition3(x: int) {
				x < 100
			}`,
			expectedOutput: `digraph {
graph [
rankdir=BT
];

// Node definitions.
2 [label="document#admin"];
3 [label=" user[with condition1]"];
4 [label="document#viewer"];
5 [label=" user[with condition3]:*"];
6 [label="document#writer"];
7 [label=" user[with condition2]"];

// Edge definitions.
3 -> 2 [
label=1
headlabel=""
];
5 -> 4 [
label=2
headlabel=""
];
7 -> 6 [
label=3
headlabel=""
];
}`,
		},
		`multigraph`: {
			inputModel: `
				model
				  schema 1.1
				
				type user
				
				type state
				  relations
					define can_view: [user]
				
				type transition
				  relations
					define start: [state]
					define end: [state]
					define can_apply: [user] and can_view from start and can_view from end`,
			expectedOutput: `digraph {
graph [
rankdir=BT
];

// Node definitions.
0 [label=state];
2 [label="state#can_view"];
3 [label=user];
6 [label="transition#can_apply"];
7 [label="transition#end"];
8 [label="transition#start"];

// Edge definitions.
0 -> 7 [
label=5
headlabel=""
];
0 -> 8 [
headlabel=""
label=6
];
2 -> 6 [
label=3
headlabel="(transition#start)"
];
2 -> 6 [
label=4
headlabel="(transition#end)"
];
3 -> 2 [
label=1
headlabel=""
];
3 -> 6 [
label=2
headlabel=""
];
}`,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			actual := Writer(test.inputModel)
			actualSorted := getSorted(actual)
			expectedSorted := getSorted(test.expectedOutput)
			diff := cmp.Diff(expectedSorted, actualSorted)

			require.Empty(t, diff, "expected %s, got %s", test.expectedOutput, actual)
		})
	}
}

// getSorted assumes the input has multiple lines and returns the sorted version of it.
func getSorted(input string) string {
	lines := strings.FieldsFunc(input, func(r rune) bool {
		return r == '\n'
	})

	sort.Strings(lines)

	return strings.Join(lines, "\n")
}