package demo

import (
	"context"
	"testing"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

func TestLoadProvidesScreenshotScenario(t *testing.T) {
	result, err := New().Load(context.Background(), graph.SearchOptions{Authored: true, Assigned: true, ReviewRequested: true})
	if err != nil {
		t.Fatal(err)
	}
	prs, repos, branches := 0, 0, 0
	relations := map[string]int{}
	reReviewPR := 0
	for _, node := range result.Nodes {
		if node.Kind == "repository" {
			repos++
		} else if node.Kind == "branch" {
			branches++
			if node.Branch.Name != "release/2026-q3" {
				t.Fatalf("branch name = %q, want release/2026-q3", node.Branch.Name)
			}
		} else if node.Kind == "pullRequest" {
			prs++
			relations[node.PR.Relation]++
			if node.PR.ReReviewRequested {
				reReviewPR = node.PR.Number
			}
		}
	}
	if repos != 2 || prs != 5 {
		t.Fatalf("repositories/PRs = %d/%d, want 2/5", repos, prs)
	}
	if branches != 1 {
		t.Fatalf("branches = %d, want 1", branches)
	}
	dashedBranchEdge := false
	for _, edge := range result.Edges {
		if edge.Source == "repo:atlas" && edge.Target == "branch:atlas:release/2026-q3" && edge.Dashed {
			dashedBranchEdge = true
		}
	}
	if !dashedBranchEdge {
		t.Fatal("missing dashed repository-to-branch edge")
	}
	if relations["mine"] != 1 || relations["assigned"] != 1 || relations["review-requested"] != 2 || relations["other"] != 1 {
		t.Fatalf("relations = %+v", relations)
	}
	if reReviewPR != 217 {
		t.Fatalf("re-review PR = %d, want 217", reReviewPR)
	}
}
