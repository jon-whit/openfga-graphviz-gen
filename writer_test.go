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
			expectedOutput: `strict digraph {
	rankdir="BT";
	"group#member" [  weight=0 ];
	"group#member" -> "document#viewer" [ label="5",  weight=0 ];
	"group#member" -> "group#member" [ label="10",  weight=0 ];
	"folder" [  weight=0 ];
	"folder" -> "document#parent" [ label="2",  weight=0 ];
	"folder#viewer" [  weight=0 ];
	"folder#viewer" -> "document#viewer" [ headlabel="(document#parent)", label="7",  weight=0 ];
	"document#editor" [  weight=0 ];
	"document#editor" -> "document#viewer" [ label="6",  weight=0 ];
	"document#viewer" [  weight=0 ];
	"user:*" [  weight=0 ];
	"user:*" -> "document#viewer" [ label="4",  weight=0 ];
	"user" [  weight=0 ];
	"user" -> "document#editor" [ label="1",  weight=0 ];
	"user" -> "document#viewer" [ label="3",  weight=0 ];
	"user" -> "folder#viewer" [ label="8",  weight=0 ];
	"user" -> "group#member" [ label="9",  weight=0 ];
	"document#parent" [  weight=0 ];
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
			expectedOutput: `strict digraph {
	rankdir="BT";
	"document#c" [  weight=0 ];
	"document#a" [  weight=0 ];
	"document#a" -> "document#c" [ label="3", style="dashed",  weight=0 ];
	"user" [  weight=0 ];
	"user" -> "document#a" [ label="1",  weight=0 ];
	"user" -> "document#b" [ label="2",  weight=0 ];
	"document#b" [  weight=0 ];
	"document#b" -> "document#c" [ label="4", style="dashed",  weight=0 ];
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
			expectedOutput: `strict digraph {
	rankdir="BT";
	"document#c" [  weight=0 ];
	"document#a" [  weight=0 ];
	"document#a" -> "document#c" [ label="3",  weight=0 ];
	"user" [  weight=0 ];
	"user" -> "document#a" [ label="1",  weight=0 ];
	"user" -> "document#b" [ label="2",  weight=0 ];
	"document#b" [  weight=0 ];
	"document#b" -> "document#c" [ label="4",  weight=0 ];
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
			expectedOutput: `strict digraph {
	rankdir="BT";
	" user[with condition2]" [  weight=0 ];
	" user[with condition2]" -> "document#writer" [ label="3",  weight=0 ];
	"document#writer" [  weight=0 ];
	"document#admin" [  weight=0 ];
	" user[with condition1]" [  weight=0 ];
	" user[with condition1]" -> "document#admin" [ label="1",  weight=0 ];
	" user[with condition3]:*" [  weight=0 ];
	" user[with condition3]:*" -> "document#viewer" [ label="2",  weight=0 ];
	"document#viewer" [  weight=0 ];
}`,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			edgeCounter = 0
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
