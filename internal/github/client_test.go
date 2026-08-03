package github

import (
	"strings"
	"testing"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

func TestConvertReviewSummary(t *testing.T) {
	raw := rawPR{ID: "pr1", HeadRefOid: "head-sha", BaseRefOid: "base-sha", Author: &rawUser{Login: "dependabot[bot]", Typename: "Bot"}}
	raw.LatestReviews.Nodes = append(raw.LatestReviews.Nodes,
		struct {
			State  string
			Author *rawUser
		}{State: "APPROVED", Author: &rawUser{Login: "alice"}},
		struct {
			State  string
			Author *rawUser
		}{State: "CHANGES_REQUESTED", Author: &rawUser{Login: "bob"}},
	)
	raw.ReviewRequests.Nodes = append(raw.ReviewRequests.Nodes, struct {
		RequestedReviewer struct {
			Typename string `json:"__typename"`
			Login    string
			Slug     string
		}
	}{})
	raw.ReviewRequests.Nodes[0].RequestedReviewer.Typename = "User"
	raw.ReviewRequests.Nodes[0].RequestedReviewer.Login = "carol"

	got := convert(raw)
	if got.ReviewApproved != 1 || got.ReviewTotal != 3 {
		t.Fatalf("review summary = %d/%d, want 1/3", got.ReviewApproved, got.ReviewTotal)
	}
	if got.HeadCommitSHA != "head-sha" || got.BaseCommitSHA != "base-sha" {
		t.Fatalf("commit SHAs = %q/%q, want head-sha/base-sha", got.HeadCommitSHA, got.BaseCommitSHA)
	}
	if !got.IsBot {
		t.Fatal("bot author was not detected")
	}
}

func TestMergedPRNumbers(t *testing.T) {
	message := "Merge pull request #42 from org/feature\n\nMerged change #43"
	got := mergedPRNumbers(message, 99)
	if len(got) != 2 || got[0] != 42 || got[1] != 43 {
		t.Fatalf("merged PR numbers = %v, want [42 43]", got)
	}
}

func TestMergedPRNumbersExcludesCurrentPR(t *testing.T) {
	got := mergedPRNumbers("Merge pull request #42 from org/feature", 42)
	if len(got) != 0 {
		t.Fatalf("merged PR numbers = %v, want none", got)
	}
}

func TestBuildIncludedPullRequestsQueryBatchesCandidates(t *testing.T) {
	parents := map[includedCandidate][]string{
		{repository: "orangain/one", number: 12}: {"parent1"},
		{repository: "orangain/two", number: 34}: {"parent2"},
	}
	query, aliases := buildIncludedPullRequestsQuery(parents)
	if len(aliases) != 2 {
		t.Fatalf("aliases = %d, want 2", len(aliases))
	}
	if strings.Count(query, "query{") != 1 || strings.Count(query, "pullRequest(number:") != 2 {
		t.Fatalf("expected one batched query containing two pull requests: %s", query)
	}
}

func TestBuildDownstreamQueryBatchesBranches(t *testing.T) {
	targets := []downstreamQueryTarget{
		{owner: "orangain", name: "one", base: "feature/a"},
		{owner: "orangain", name: "two", base: "feature/b"},
	}
	query, aliases := buildDownstreamQuery(targets)
	if len(aliases) != 2 {
		t.Fatalf("aliases = %d, want 2", len(aliases))
	}
	if strings.Count(query, "query{") != 1 || strings.Count(query, "pullRequests(first:100") != 2 {
		t.Fatalf("expected one batched query containing two branches: %s", query)
	}
	if !strings.Contains(query, `baseRefName:"feature/a"`) || !strings.Contains(query, `baseRefName:"feature/b"`) {
		t.Fatalf("query does not contain both base branches: %s", query)
	}
}

func TestBuildUpstreamQueryBatchesBranches(t *testing.T) {
	targets := []upstreamQueryTarget{
		{owner: "orangain", name: "one", head: "feature/a"},
		{owner: "orangain", name: "two", head: "feature/b"},
	}
	query, aliases := buildUpstreamQuery(targets)
	if len(aliases) != 2 {
		t.Fatalf("aliases = %d, want 2", len(aliases))
	}
	if strings.Count(query, "query{") != 1 || strings.Count(query, "pullRequests(first:100") != 2 {
		t.Fatalf("expected one batched query containing two branches: %s", query)
	}
	if !strings.Contains(query, `headRefName:"feature/a"`) || !strings.Contains(query, `headRefName:"feature/b"`) {
		t.Fatalf("query does not contain both head branches: %s", query)
	}
	if strings.Contains(query, "baseRefName:") {
		t.Fatalf("upstream query unexpectedly filters base branches: %s", query)
	}
}

func TestIsUpstreamParentRequiresExactRepositoryAndBranch(t *testing.T) {
	child := &graph.PullRequest{RepositoryID: "base-repo", BaseRefName: "feature/a"}
	if !isUpstreamParent(&graph.PullRequest{HeadRepositoryID: "base-repo", HeadRefName: "feature/a"}, child) {
		t.Fatal("matching repository and branch were not recognized as upstream")
	}
	if isUpstreamParent(&graph.PullRequest{HeadRepositoryID: "fork-repo", HeadRefName: "feature/a"}, child) {
		t.Fatal("same-named branch from a fork was recognized as upstream")
	}
}

func TestBuildSearchSpecsTreatsQueryAsFourthORCondition(t *testing.T) {
	specs := buildSearchSpecs(graph.SearchOptions{Authored: true, Assigned: true, ReviewRequested: true, Query: "repo:orangain/example"})
	if len(specs) != 4 || specs[3].query != "repo:orangain/example" {
		t.Fatalf("specs = %+v, want three relationship queries plus custom query", specs)
	}
	specs = buildSearchSpecs(graph.SearchOptions{Query: "repo:orangain/example"})
	if len(specs) != 1 || specs[0].query != "repo:orangain/example" {
		t.Fatalf("specs = %+v, want only custom query", specs)
	}
}
