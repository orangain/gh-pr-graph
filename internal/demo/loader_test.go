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
	reReviewPRs := map[int]bool{}
	pendingReviewPR := 0
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
				reReviewPRs[node.PR.Number] = true
			}
			if node.PR.ViewerPendingReview {
				pendingReviewPR = node.PR.Number
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
	if !reReviewPRs[217] || !reReviewPRs[221] {
		t.Fatalf("re-review PRs = %+v, want 217 and 221", reReviewPRs)
	}
	if pendingReviewPR != 221 {
		t.Fatalf("pending-review PR = %d, want 221", pendingReviewPR)
	}
}
