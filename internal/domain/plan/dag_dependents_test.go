package plan_test

import (
	"slices"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

func TestDependentSteps(t *testing.T) {
	steps := []plan.Step{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"b"}},
		{ID: "d", DependsOn: []string{"x", "c"}},
		{ID: "e", DependsOn: []string{"a", "b"}},
		{ID: "x"},
		{ID: "y", DependsOn: []string{"x"}},
	}
	tests := []struct {
		name string
		step string
		want []string
	}{
		{name: "direct and transitive, each once, in plan order", step: "a", want: []string{"b", "c", "d", "e"}},
		{name: "middle of a chain", step: "c", want: []string{"d"}},
		{name: "several roots", step: "x", want: []string{"d", "y"}},
		{name: "leaf", step: "d"},
		{name: "unknown step", step: "zzz"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan.DependentSteps(steps, tc.step); !slices.Equal(got, tc.want) {
				t.Errorf("DependentSteps(%s) = %v, want %v", tc.step, got, tc.want)
			}
		})
	}

	t.Run("a cycle ends", func(t *testing.T) {
		cyclic := []plan.Step{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}
		if got := plan.DependentSteps(cyclic, "a"); !slices.Equal(got, []string{"a", "b"}) {
			t.Errorf("DependentSteps = %v, want [a b]", got)
		}
	})
}
