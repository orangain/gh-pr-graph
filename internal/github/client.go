package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

const commitMessagesQuery = `query($id:ID!,$after:String){
  node(id:$id){... on PullRequest{
    commits(first:100,after:$after){
      pageInfo{hasNextPage endCursor}
      nodes{commit{messageHeadline messageBody}}
    }
  }}
}`

const pullRequestByNumberQuery = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){pullRequest(number:$number){
    id number title url merged mergedAt author{login avatarUrl}
  }}
}`

var mergedPRPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^Merge pull request #(\d+)\b`),
	regexp.MustCompile(`(?im)^Merged?[^\n#]*#(\d+)\b`),
}

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

type commitMessagesResponse struct {
	Data struct {
		Node *struct {
			Commits struct {
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
				Nodes []struct {
					Commit struct{ MessageHeadline, MessageBody string }
				}
			}
		}
	}
}

type pullRequestByNumberResponse struct {
	Data struct {
		Repository *struct {
			PullRequest *struct {
				ID, Title, URL string
				Number         int
				Merged         bool
				MergedAt       *time.Time
				Author         *rawUser
			}
		}
	}
}

func (c *Client) Load(ctx context.Context, query string) (graph.Result, error) {
	return c.LoadProgress(ctx, query, nil)
}

func (c *Client) LoadProgress(ctx context.Context, query string, progress func(current, total int, phase string)) (graph.Result, error) {
	report := func(current, total int, phase string) {
		if progress != nil {
			progress(current, total, phase)
		}
	}
	queries := []string{query}
	if strings.TrimSpace(query) == "" {
		queries = []string{"is:pr is:open author:@me", "is:pr is:open assignee:@me", "is:pr is:open review-requested:@me"}
	}
	byID := map[string]*graph.PullRequest{}
	reviewSeeds := map[string]bool{}
	viewer := ""
	warnings := []string{}
	report(0, len(queries), "Searching pull requests")
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
		report(i+1, len(queries), "Searching pull requests")
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
	discovered := 0
	report(0, len(queue), "Discovering stacked pull requests")
	reportDiscovery := func() { report(discovered, len(byID), "Discovering stacked pull requests") }
	for len(queue) > 0 && len(byID) < c.MaxPRs {
		current := queue[0]
		queue = queue[1:]
		discovered++
		if current.depth >= c.MaxDepth || current.pr.HeadRepository == "" || current.pr.HeadRefName == "" {
			reportDiscovery()
			continue
		}
		key := current.pr.HeadRepositoryID + "\x00" + current.pr.HeadRefName
		if visitedRefs[key] {
			reportDiscovery()
			continue
		}
		visitedRefs[key] = true
		parts := strings.SplitN(current.pr.HeadRepository, "/", 2)
		if len(parts) != 2 {
			reportDiscovery()
			continue
		}
		var response downstreamResponse
		err := c.graphql(ctx, downstreamQuery, map[string]string{"owner": parts[0], "name": parts[1], "base": current.pr.HeadRefName}, &response)
		if err != nil {
			warnings = append(warnings, "Could not discover downstream PRs for "+current.pr.HeadRepository+":"+current.pr.HeadRefName)
			reportDiscovery()
			continue
		}
		if response.Data.Repository == nil {
			reportDiscovery()
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
		reportDiscovery()
	}
	if len(byID) >= c.MaxPRs {
		warnings = append(warnings, "PR limit reached; narrow the search to see the complete graph.")
	}
	prs := make([]*graph.PullRequest, 0, len(byID))
	for _, pr := range byID {
		prs = append(prs, pr)
	}
	if err := c.loadIncludedFromMessages(ctx, prs, report); err != nil {
		warnings = append(warnings, "Some included pull requests could not be loaded: "+err.Error())
	}
	return graph.Build(prs, warnings), nil
}

func (c *Client) loadIncludedFromMessages(ctx context.Context, prs []*graph.PullRequest, report func(int, int, string)) error {
	if len(prs) == 0 {
		report(1, 1, "Inspecting included pull requests")
		return nil
	}
	type job struct{ pr *graph.PullRequest }
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0
	errorsSeen := []string{}
	workers := 6
	if len(prs) < workers {
		workers = len(prs)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				included, err := c.includedFromMessages(ctx, item.pr)
				mu.Lock()
				if err != nil {
					errorsSeen = append(errorsSeen, err.Error())
				} else {
					item.pr.IncludedPRs = included
				}
				completed++
				report(completed, len(prs), "Inspecting included pull requests")
				mu.Unlock()
			}
		}()
	}
	for _, pr := range prs {
		jobs <- job{pr: pr}
	}
	close(jobs)
	wg.Wait()
	if len(errorsSeen) > 0 {
		return fmt.Errorf("%d of %d commit scans failed", len(errorsSeen), len(prs))
	}
	return nil
}

func (c *Client) includedFromMessages(ctx context.Context, pr *graph.PullRequest) ([]graph.IncludedPullRequest, error) {
	numbers := map[int]bool{}
	after := ""
	for page := 0; page < 3; page++ {
		variables := map[string]string{"id": pr.ID}
		if after != "" {
			variables["after"] = after
		}
		var response commitMessagesResponse
		if err := c.graphql(ctx, commitMessagesQuery, variables, &response); err != nil {
			return nil, err
		}
		if response.Data.Node == nil {
			return nil, fmt.Errorf("pull request %s not found", pr.ID)
		}
		for _, node := range response.Data.Node.Commits.Nodes {
			message := node.Commit.MessageHeadline + "\n" + node.Commit.MessageBody
			for _, number := range mergedPRNumbers(message, pr.Number) {
				numbers[number] = true
			}
		}
		pageInfo := response.Data.Node.Commits.PageInfo
		if !pageInfo.HasNextPage {
			break
		}
		after = pageInfo.EndCursor
	}
	if len(numbers) == 0 {
		return nil, nil
	}
	parts := strings.SplitN(pr.Repository, "/", 2)
	if len(parts) != 2 {
		return nil, nil
	}
	result := []graph.IncludedPullRequest{}
	orderedNumbers := make([]int, 0, len(numbers))
	for number := range numbers {
		orderedNumbers = append(orderedNumbers, number)
	}
	sort.Ints(orderedNumbers)
	for _, number := range orderedNumbers {
		var response pullRequestByNumberResponse
		variables := map[string]string{"owner": parts[0], "name": parts[1], "number": strconv.Itoa(number)}
		if err := c.graphql(ctx, pullRequestByNumberQuery, variables, &response); err != nil {
			return nil, err
		}
		if response.Data.Repository == nil || response.Data.Repository.PullRequest == nil || !response.Data.Repository.PullRequest.Merged {
			continue
		}
		raw := response.Data.Repository.PullRequest
		included := graph.IncludedPullRequest{ID: raw.ID, Number: raw.Number, Title: raw.Title, URL: raw.URL, MergedAt: raw.MergedAt}
		if raw.Author != nil {
			included.Author = graph.User{Login: raw.Author.Login, AvatarURL: raw.Author.AvatarURL}
		}
		result = append(result, included)
	}
	return result, nil
}

func mergedPRNumbers(message string, currentPR int) []int {
	found := map[int]bool{}
	for _, pattern := range mergedPRPatterns {
		for _, match := range pattern.FindAllStringSubmatch(message, -1) {
			number, _ := strconv.Atoi(match[1])
			if number > 0 && number != currentPR {
				found[number] = true
			}
		}
	}
	numbers := make([]int, 0, len(found))
	for number := range found {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	return numbers
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
