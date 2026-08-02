package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

const searchQuery = `query($q:String!){
  viewer{login}
  search(query:$q,type:ISSUE,first:100){
    issueCount
    nodes{... on PullRequest{` + prFields + `}}
  }
}`

const downstreamQuery = `query($owner:String!,$name:String!,$base:String!){
  repository(owner:$owner,name:$name){
    pullRequests(first:100,states:OPEN,baseRefName:$base){nodes{` + prFields + `}}
  }
}`

const includedQuery = `query($id:ID!,$after:String){
  node(id:$id){... on PullRequest{
    id
    commits(first:100,after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{commit{
        oid
        associatedPullRequests(first:10){
          pageInfo{hasNextPage}
          nodes{id number title url merged mergedAt author{login avatarUrl} mergeCommit{oid}}
        }
      }}
    }
  }}
}`

const prFields = `
  id number title url isDraft updatedAt baseRefName headRefName reviewDecision mergeable
  author{login avatarUrl}
  repository{id nameWithOwner defaultBranchRef{name}}
  headRepository{id nameWithOwner}
  assignees(first:20){nodes{login avatarUrl}}
  reviewRequests(first:50){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug}}}}
  latestReviews(first:50){nodes{state author{login}}}
  commits(last:1){nodes{commit{statusCheckRollup{state}}}}
`

type Client struct {
	Hostname string
	MaxPRs   int
	MaxDepth int
}

func New(hostname string) *Client { return &Client{Hostname: hostname, MaxPRs: 500, MaxDepth: 20} }

type rawUser struct{ Login, AvatarURL string }
type rawPR struct {
	ID, Title, URL, BaseRefName, HeadRefName, ReviewDecision, Mergeable string
	Number                                                              int
	IsDraft                                                             bool
	UpdatedAt                                                           time.Time
	Author                                                              *rawUser
	Repository                                                          struct {
		ID, NameWithOwner string
		DefaultBranchRef  *struct{ Name string }
	}
	HeadRepository *struct{ ID, NameWithOwner string }
	Assignees      struct{ Nodes []rawUser }
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Typename    string `json:"__typename"`
				Login, Slug string
			}
		}
	}
	LatestReviews struct {
		Nodes []struct {
			State  string
			Author *rawUser
		}
	}
	Commits struct {
		Nodes []struct {
			Commit struct{ StatusCheckRollup *struct{ State string } }
		}
	}
}

type searchResponse struct {
	Data struct {
		Viewer rawUser
		Search struct {
			IssueCount int
			Nodes      []rawPR
		}
	}
}
type downstreamResponse struct {
	Data struct {
		Repository *struct{ PullRequests struct{ Nodes []rawPR } }
	}
}

type includedResponse struct {
	Data struct {
		Node *struct {
			ID      string
			Commits struct {
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
				Nodes []struct {
					Commit struct {
						OID                    string
						AssociatedPullRequests struct {
							PageInfo struct{ HasNextPage bool }
							Nodes    []struct {
								ID, Title, URL string
								Number         int
								Merged         bool
								MergedAt       *time.Time
								Author         *rawUser
								MergeCommit    *struct{ OID string }
							}
						}
					}
				}
			}
		}
	}
}

func (c *Client) Load(ctx context.Context, query string) (graph.Result, error) {
	queries := []string{query}
	if strings.TrimSpace(query) == "" {
		queries = []string{"is:pr is:open author:@me", "is:pr is:open assignee:@me", "is:pr is:open review-requested:@me"}
	}
	byID := map[string]*graph.PullRequest{}
	reviewSeeds := map[string]bool{}
	viewer := ""
	warnings := []string{}
	for i, q := range queries {
		var response searchResponse
		if err := c.graphql(ctx, searchQuery, map[string]string{"q": q}, &response); err != nil {
			return graph.Result{}, err
		}
		if viewer == "" {
			viewer = response.Data.Viewer.Login
		}
		if response.Data.Search.IssueCount > len(response.Data.Search.Nodes) {
			warnings = append(warnings, fmt.Sprintf("Search %q has more than 100 results; showing the first 100.", q))
		}
		for _, raw := range response.Data.Search.Nodes {
			pr := convert(raw)
			if pr == nil {
				continue
			}
			if i == 2 && query == "" {
				reviewSeeds[pr.ID] = true
			}
			byID[pr.ID] = pr
		}
	}
	for id, pr := range byID {
		pr.Relation = graph.RelationFor(pr, viewer, reviewSeeds[id])
		pr.Source = "search"
	}

	type item struct {
		pr    *graph.PullRequest
		depth int
	}
	queue := make([]item, 0, len(byID))
	for _, pr := range byID {
		queue = append(queue, item{pr, 0})
	}
	visitedRefs := map[string]bool{}
	for len(queue) > 0 && len(byID) < c.MaxPRs {
		current := queue[0]
		queue = queue[1:]
		if current.depth >= c.MaxDepth || current.pr.HeadRepository == "" || current.pr.HeadRefName == "" {
			continue
		}
		key := current.pr.HeadRepositoryID + "\x00" + current.pr.HeadRefName
		if visitedRefs[key] {
			continue
		}
		visitedRefs[key] = true
		parts := strings.SplitN(current.pr.HeadRepository, "/", 2)
		if len(parts) != 2 {
			continue
		}
		var response downstreamResponse
		err := c.graphql(ctx, downstreamQuery, map[string]string{"owner": parts[0], "name": parts[1], "base": current.pr.HeadRefName}, &response)
		if err != nil {
			warnings = append(warnings, "Could not discover downstream PRs for "+current.pr.HeadRepository+":"+current.pr.HeadRefName)
			continue
		}
		if response.Data.Repository == nil {
			continue
		}
		for _, raw := range response.Data.Repository.PullRequests.Nodes {
			if len(byID) >= c.MaxPRs {
				break
			}
			pr := convert(raw)
			if pr == nil {
				continue
			}
			if _, exists := byID[pr.ID]; exists {
				continue
			}
			pr.Relation = graph.RelationFor(pr, viewer, hasViewerRequest(raw, viewer))
			pr.Source = "downstream"
			byID[pr.ID] = pr
			queue = append(queue, item{pr, current.depth + 1})
		}
	}
	if len(byID) >= c.MaxPRs {
		warnings = append(warnings, "PR limit reached; narrow the search to see the complete graph.")
	}
	prs := make([]*graph.PullRequest, 0, len(byID))
	for _, pr := range byID {
		prs = append(prs, pr)
	}
	return graph.Build(prs, warnings), nil
}

func (c *Client) Included(ctx context.Context, id string) (graph.IncludedResult, error) {
	result := graph.IncludedResult{}
	seen := map[string]bool{}
	after := ""
	for page := 0; page < 3; page++ {
		variables := map[string]string{"id": id}
		if after != "" {
			variables["after"] = after
		}
		var response includedResponse
		if err := c.graphql(ctx, includedQuery, variables, &response); err != nil {
			return graph.IncludedResult{}, err
		}
		if response.Data.Node == nil {
			return graph.IncludedResult{}, fmt.Errorf("pull request not found")
		}
		for _, commitNode := range response.Data.Node.Commits.Nodes {
			associated := commitNode.Commit.AssociatedPullRequests
			if associated.PageInfo.HasNextPage {
				result.Truncated = true
			}
			for _, candidate := range associated.Nodes {
				// Only count an exact merge commit contained in this PR's commit set.
				if candidate.ID == id || !candidate.Merged || candidate.MergeCommit == nil || candidate.MergeCommit.OID != commitNode.Commit.OID || seen[candidate.ID] {
					continue
				}
				seen[candidate.ID] = true
				included := graph.IncludedPullRequest{ID: candidate.ID, Number: candidate.Number, Title: candidate.Title, URL: candidate.URL, MergedAt: candidate.MergedAt}
				if candidate.Author != nil {
					included.Author = graph.User{Login: candidate.Author.Login, AvatarURL: candidate.Author.AvatarURL}
				}
				result.PullRequests = append(result.PullRequests, included)
			}
		}
		pageInfo := response.Data.Node.Commits.PageInfo
		if !pageInfo.HasNextPage {
			return result, nil
		}
		after = pageInfo.EndCursor
	}
	result.Truncated = true
	return result, nil
}

func convert(raw rawPR) *graph.PullRequest {
	if raw.ID == "" {
		return nil
	}
	pr := &graph.PullRequest{ID: raw.ID, Number: raw.Number, Title: raw.Title, URL: raw.URL, IsDraft: raw.IsDraft, UpdatedAt: raw.UpdatedAt, BaseRefName: raw.BaseRefName, HeadRefName: raw.HeadRefName, ReviewDecision: raw.ReviewDecision, Mergeable: raw.Mergeable}
	if raw.Author != nil {
		pr.Author = graph.User{Login: raw.Author.Login, AvatarURL: raw.Author.AvatarURL}
	}
	pr.RepositoryID, pr.Repository = raw.Repository.ID, raw.Repository.NameWithOwner
	if raw.Repository.DefaultBranchRef != nil {
		pr.DefaultBranch = raw.Repository.DefaultBranchRef.Name
	}
	if raw.HeadRepository != nil {
		pr.HeadRepositoryID, pr.HeadRepository = raw.HeadRepository.ID, raw.HeadRepository.NameWithOwner
	}
	for _, user := range raw.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, graph.User{Login: user.Login, AvatarURL: user.AvatarURL})
	}
	latest := map[string]string{}
	for _, review := range raw.LatestReviews.Nodes {
		if review.Author != nil {
			latest[review.Author.Login] = review.State
		}
	}
	pending := map[string]bool{}
	for _, req := range raw.ReviewRequests.Nodes {
		if req.RequestedReviewer.Typename == "User" {
			pending[req.RequestedReviewer.Login] = true
		}
		if req.RequestedReviewer.Typename == "Team" {
			pr.TeamReviewPending = true
			pending["team:"+req.RequestedReviewer.Slug] = true
		}
	}
	for login, state := range latest {
		if state == "APPROVED" {
			pr.ReviewApproved++
		}
		pending[login] = true
	}
	pr.ReviewTotal = len(pending)
	if len(raw.Commits.Nodes) > 0 && raw.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		pr.CIState = raw.Commits.Nodes[0].Commit.StatusCheckRollup.State
	}
	return pr
}

func hasViewerRequest(raw rawPR, viewer string) bool {
	for _, req := range raw.ReviewRequests.Nodes {
		if req.RequestedReviewer.Typename == "User" && req.RequestedReviewer.Login == viewer {
			return true
		}
	}
	return false
}

func (c *Client) graphql(ctx context.Context, query string, variables map[string]string, target any) error {
	args := []string{"api", "graphql"}
	if c.Hostname != "" {
		args = append(args, "--hostname", c.Hostname)
	}
	args = append(args, "-f", "query="+query)
	for key, value := range variables {
		args = append(args, "-F", key+"="+value)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("GitHub API: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}
