package graph

import "testing"

func TestBuildStack(t *testing.T) {
	prs := []*PullRequest{
		{ID: "1", Number: 1, RepositoryID: "r", Repository: "o/r", RepositoryURL: "https://github.com/o/r", DefaultBranch: "main", BaseRefName: "main", HeadRepositoryID: "r", HeadRefName: "a"},
		{ID: "2", Number: 2, RepositoryID: "r", Repository: "o/r", DefaultBranch: "main", BaseRefName: "a", HeadRepositoryID: "r", HeadRefName: "b"},
	}
	got := Build(prs, nil)
	if len(got.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(got.Edges))
	}
	if got.Nodes[2].Rank != 2 {
		t.Fatalf("rank = %d, want 2", got.Nodes[2].Rank)
	}
	if got.Nodes[0].Repo.URL != "https://github.com/o/r" {
		t.Fatalf("repository URL = %q", got.Nodes[0].Repo.URL)
	}
}

func TestBuildCreatesNodeForUnknownNonDefaultBase(t *testing.T) {
	prs := []*PullRequest{{ID: "1", RepositoryID: "r", Repository: "o/r", RepositoryURL: "https://github.com/o/r", DefaultBranch: "main", BaseRefName: "release/very long-branch"}}
	got := Build(prs, nil)
	if len(got.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(got.Edges))
	}
	branchID := branchNodeID("r", "release/very long-branch")
	if got.Nodes[1].Kind != "branch" || got.Nodes[1].ID != branchID || got.Nodes[1].Branch.Name != "release/very long-branch" {
		t.Fatalf("branch node = %+v", got.Nodes[1])
	}
	if got.Nodes[1].Branch.URL != "https://github.com/o/r/tree/release/very%20long-branch" {
		t.Fatalf("branch URL = %q", got.Nodes[1].Branch.URL)
	}
	if got.Nodes[2].Rank != 2 {
		t.Fatalf("PR rank = %d, want 2", got.Nodes[2].Rank)
	}
	if got.Edges[0].Source != "repo:r" || got.Edges[0].Target != branchID || !got.Edges[0].Dashed {
		t.Fatalf("repository edge = %+v", got.Edges[0])
	}
	if got.Edges[1].Source != branchID || got.Edges[1].Target != "pr:1" || got.Edges[1].Dashed {
		t.Fatalf("branch edge = %+v", got.Edges[1])
	}
}

func TestBuildSharesUnknownBaseBranchNode(t *testing.T) {
	prs := []*PullRequest{
		{ID: "1", Number: 1, RepositoryID: "r", Repository: "o/r", DefaultBranch: "main", BaseRefName: "release"},
		{ID: "2", Number: 2, RepositoryID: "r", Repository: "o/r", DefaultBranch: "main", BaseRefName: "release"},
	}
	got := Build(prs, nil)
	branches := 0
	for _, node := range got.Nodes {
		if node.Kind == "branch" {
			branches++
		}
	}
	if branches != 1 || len(got.Edges) != 3 {
		t.Fatalf("branches/edges = %d/%d, want 1/3", branches, len(got.Edges))
	}
}

func TestRelationPriority(t *testing.T) {
	tests := []struct {
		name     string
		pr       *PullRequest
		review   bool
		relation string
	}{
		{name: "author takes priority", pr: &PullRequest{Author: User{Login: "me"}, Assignees: []User{{Login: "me"}}}, review: true, relation: "mine"},
		{name: "assignee takes priority over review", pr: &PullRequest{Assignees: []User{{Login: "me"}}}, review: true, relation: "assigned"},
		{name: "review requested", pr: &PullRequest{}, review: true, relation: "review-requested"},
		{name: "other", pr: &PullRequest{}, relation: "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelationFor(tt.pr, "me", tt.review); got != tt.relation {
				t.Fatalf("got %q, want %q", got, tt.relation)
			}
		})
	}
}
