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

func TestBuildConnectsUnknownNonDefaultBaseToRepository(t *testing.T) {
	prs := []*PullRequest{{ID: "1", RepositoryID: "r", Repository: "o/r", DefaultBranch: "main", BaseRefName: "release/very-long-branch"}}
	got := Build(prs, nil)
	if len(got.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(got.Edges))
	}
	edge := got.Edges[0]
	if edge.Source != "repo:r" || edge.Target != "pr:1" || edge.Label != "release/very-long-branch" || !edge.Dashed {
		t.Fatalf("edge = %+v", edge)
	}
}

func TestRelationPriority(t *testing.T) {
	pr := &PullRequest{Author: User{Login: "me"}}
	if got := RelationFor(pr, "me", true); got != "mine" {
		t.Fatalf("got %q", got)
	}
}
