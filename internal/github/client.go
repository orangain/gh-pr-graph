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
	"github.com/orangain/gh-pr-graph/internal/oteltrace"
)

const maxConcurrentGitHubRequests = 6

const searchQuery = `query($q:String!){
  viewer{login}
  search(query:$q,type:ISSUE,first:100){
    issueCount
    nodes{... on PullRequest{` + prFields + `}}
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

var mergedPRPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^Merge pull request #(\d+)\b`),
	regexp.MustCompile(`(?im)^Merged?[^\n#]*#(\d+)\b`),
}

const prFields = `
  id number title url isDraft updatedAt baseRefName headRefName baseRefOid headRefOid reviewDecision mergeable
  author{__typename login avatarUrl}
  repository{id nameWithOwner url defaultBranchRef{name}}
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
	Tracer   oteltrace.Tracer
}

func New(hostname string) *Client { return &Client{Hostname: hostname, MaxPRs: 500, MaxDepth: 20} }

type rawUser struct {
	Login, AvatarURL string
	Typename         string `json:"__typename"`
}
type rawPR struct {
	ID, Title, URL, BaseRefName, HeadRefName, BaseRefOid, HeadRefOid, ReviewDecision, Mergeable string
	Number                                                                                      int
	IsDraft                                                                                     bool
	UpdatedAt                                                                                   time.Time
	Author                                                                                      *rawUser
	Repository                                                                                  struct {
		ID, NameWithOwner, URL string
		DefaultBranchRef       *struct{ Name string }
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
type batchDownstreamResponse struct{ Data map[string]json.RawMessage }

type rawDownstreamRepository struct {
	PullRequests struct{ Nodes []rawPR }
}

type downstreamQueryTarget struct{ owner, name, base string }
type upstreamQueryTarget struct{ owner, name, head string }

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

type rawIncludedPR struct {
	ID, Title, URL string
	Number         int
	MergedAt       *time.Time
	Author         *rawUser
}

type batchPullRequestResponse struct {
	Data map[string]json.RawMessage
}

type includedCandidate struct {
	repository string
	number     int
}

type searchSpec struct {
	query         string
	reviewRequest bool
}

func buildSearchSpecs(options graph.SearchOptions) []searchSpec {
	queries := []searchSpec{}
	if options.Authored {
		queries = append(queries, searchSpec{query: "is:pr is:open author:@me"})
	}
	if options.Assigned {
		queries = append(queries, searchSpec{query: "is:pr is:open assignee:@me"})
	}
	if options.ReviewRequested {
		queries = append(queries, searchSpec{query: "is:pr is:open review-requested:@me", reviewRequest: true})
	}
	if query := strings.TrimSpace(options.Query); query != "" {
		queries = append(queries, searchSpec{query: query})
	}
	return queries
}

func (c *Client) Load(ctx context.Context, options graph.SearchOptions) (graph.Result, error) {
	return c.LoadProgress(ctx, options, nil)
}

func (c *Client) LoadProgress(ctx context.Context, options graph.SearchOptions, progress func(current, total int, phase string)) (result graph.Result, resultErr error) {
	ctx, loadSpan := c.startSpan(ctx, "load pull request graph", oteltrace.SpanInternal, oteltrace.Attributes{"pr.search_query": options.Query})
	if loadSpan != nil {
		defer func() {
			loadSpan.End(resultErr, oteltrace.Attributes{"pr.node_count": len(result.Nodes), "pr.edge_count": len(result.Edges)})
		}()
	}
	report := func(current, total int, phase string) {
		if progress != nil {
			progress(current, total, phase)
		}
	}
	queries := buildSearchSpecs(options)
	byID := map[string]*graph.PullRequest{}
	reviewSeeds := map[string]bool{}
	viewer := ""
	warnings := []string{}
	report(0, len(queries), "Searching pull requests")
	searchCtx, searchSpan := c.startSpan(ctx, "search pull requests", oteltrace.SpanInternal, oteltrace.Attributes{"pr.query_count": len(queries)})
	type searchResult struct {
		spec     searchSpec
		response searchResponse
		err      error
	}
	searchResults := make(chan searchResult, len(queries))
	for _, spec := range queries {
		go func() {
			var response searchResponse
			err := c.graphql(searchCtx, "search pull requests", searchQuery, map[string]string{"q": spec.query}, &response)
			searchResults <- searchResult{spec: spec, response: response, err: err}
		}()
	}
	for completed := 0; completed < len(queries); completed++ {
		search := <-searchResults
		if search.err != nil {
			if searchSpan != nil {
				searchSpan.End(search.err, nil)
			}
			return graph.Result{}, search.err
		}
		response, spec := search.response, search.spec
		if viewer == "" {
			viewer = response.Data.Viewer.Login
		}
		if response.Data.Search.IssueCount > len(response.Data.Search.Nodes) {
			warnings = append(warnings, fmt.Sprintf("Search %q has more than 100 results; showing the first 100.", spec.query))
		}
		for _, raw := range response.Data.Search.Nodes {
			pr := convert(raw)
			if pr == nil {
				continue
			}
			if spec.reviewRequest || hasViewerRequest(raw, viewer) {
				reviewSeeds[pr.ID] = true
			}
			byID[pr.ID] = pr
		}
		report(completed+1, len(queries), "Searching pull requests")
	}
	if searchSpan != nil {
		searchSpan.End(nil, oteltrace.Attributes{"pr.result_count": len(byID)})
	}
	for id, pr := range byID {
		pr.Relation = graph.RelationFor(pr, viewer, reviewSeeds[id])
		pr.Source = "search"
	}
	searchSeeds := make([]*graph.PullRequest, 0, len(byID))
	for _, pr := range byID {
		searchSeeds = append(searchSeeds, pr)
	}

	type item struct {
		pr    *graph.PullRequest
		depth int
	}
	frontier := make([]item, 0, len(searchSeeds))
	for _, pr := range searchSeeds {
		frontier = append(frontier, item{pr, 0})
	}
	visitedRefs := map[string]bool{}
	discovered := 0
	report(0, len(frontier), "Discovering stacked pull requests")
	downstreamCtx, downstreamSpan := c.startSpan(ctx, "discover stacked pull requests", oteltrace.SpanInternal, oteltrace.Attributes{"pr.seed_count": len(frontier)})
	reportDiscovery := func() { report(discovered, len(byID), "Discovering stacked pull requests") }
	type downstreamJob struct {
		item  item
		owner string
		name  string
	}
	for len(frontier) > 0 && len(byID) < c.MaxPRs {
		jobs := make([]downstreamJob, 0, len(frontier))
		for _, current := range frontier {
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
			jobs = append(jobs, downstreamJob{item: current, owner: parts[0], name: parts[1]})
		}
		next := []item{}
		targets := make([]downstreamQueryTarget, len(jobs))
		for i, job := range jobs {
			targets[i] = downstreamQueryTarget{owner: job.owner, name: job.name, base: job.item.pr.HeadRefName}
		}
		query, indexesByAlias := buildDownstreamQuery(targets)
		var response batchDownstreamResponse
		if len(jobs) > 0 {
			if err := c.graphql(downstreamCtx, "batch find downstream pull requests", query, nil, &response); err != nil {
				for _, job := range jobs {
					warnings = append(warnings, "Could not discover downstream PRs for "+job.item.pr.HeadRepository+":"+job.item.pr.HeadRefName)
					reportDiscovery()
				}
				frontier = next
				continue
			}
		}
		for alias, index := range indexesByAlias {
			job := jobs[index]
			var repository *rawDownstreamRepository
			if err := json.Unmarshal(response.Data[alias], &repository); err != nil {
				warnings = append(warnings, "Could not decode downstream PRs for "+job.item.pr.HeadRepository+":"+job.item.pr.HeadRefName)
				reportDiscovery()
				continue
			}
			if repository != nil {
				for _, raw := range repository.PullRequests.Nodes {
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
					next = append(next, item{pr, job.item.depth + 1})
				}
			}
			reportDiscovery()
		}
		frontier = next
	}
	if downstreamSpan != nil {
		downstreamSpan.End(nil, oteltrace.Attributes{"pr.discovered_count": len(byID)})
	}

	frontier = make([]item, 0, len(searchSeeds))
	for _, pr := range searchSeeds {
		frontier = append(frontier, item{pr: pr})
	}
	visitedUpstreamRefs := map[string]bool{}
	upstreamCtx, upstreamSpan := c.startSpan(ctx, "discover upstream pull requests", oteltrace.SpanInternal, oteltrace.Attributes{"pr.seed_count": len(frontier)})
	type upstreamJob struct {
		item  item
		owner string
		name  string
	}
	for len(frontier) > 0 && len(byID) < c.MaxPRs {
		jobs := make([]upstreamJob, 0, len(frontier))
		for _, current := range frontier {
			if current.depth >= c.MaxDepth || current.pr.Repository == "" || current.pr.BaseRefName == "" || current.pr.BaseRefName == current.pr.DefaultBranch {
				continue
			}
			key := current.pr.RepositoryID + "\x00" + current.pr.BaseRefName
			if visitedUpstreamRefs[key] {
				continue
			}
			visitedUpstreamRefs[key] = true
			parts := strings.SplitN(current.pr.Repository, "/", 2)
			if len(parts) != 2 {
				continue
			}
			jobs = append(jobs, upstreamJob{item: current, owner: parts[0], name: parts[1]})
		}
		next := []item{}
		targets := make([]upstreamQueryTarget, len(jobs))
		for i, job := range jobs {
			targets[i] = upstreamQueryTarget{owner: job.owner, name: job.name, head: job.item.pr.BaseRefName}
		}
		query, indexesByAlias := buildUpstreamQuery(targets)
		var response batchDownstreamResponse
		if len(jobs) > 0 {
			if err := c.graphql(upstreamCtx, "batch find upstream pull requests", query, nil, &response); err != nil {
				for _, job := range jobs {
					warnings = append(warnings, "Could not discover upstream PRs for "+job.item.pr.Repository+":"+job.item.pr.BaseRefName)
				}
				frontier = next
				continue
			}
		}
		for alias, index := range indexesByAlias {
			job := jobs[index]
			var repository *rawDownstreamRepository
			if err := json.Unmarshal(response.Data[alias], &repository); err != nil {
				warnings = append(warnings, "Could not decode upstream PRs for "+job.item.pr.Repository+":"+job.item.pr.BaseRefName)
				continue
			}
			if repository == nil {
				continue
			}
			for _, raw := range repository.PullRequests.Nodes {
				if len(byID) >= c.MaxPRs {
					break
				}
				pr := convert(raw)
				if pr == nil || !isUpstreamParent(pr, job.item.pr) {
					continue
				}
				if existing, exists := byID[pr.ID]; exists {
					next = append(next, item{pr: existing, depth: job.item.depth + 1})
					continue
				}
				pr.Relation = graph.RelationFor(pr, viewer, hasViewerRequest(raw, viewer))
				pr.Source = "upstream"
				byID[pr.ID] = pr
				next = append(next, item{pr: pr, depth: job.item.depth + 1})
			}
		}
		frontier = next
	}
	if upstreamSpan != nil {
		upstreamSpan.End(nil, oteltrace.Attributes{"pr.discovered_count": len(byID)})
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

func buildDownstreamQuery(targets []downstreamQueryTarget) (string, map[string]int) {
	var query strings.Builder
	query.WriteString("query{")
	byAlias := make(map[string]int, len(targets))
	for i, target := range targets {
		alias := "r" + strconv.Itoa(i)
		byAlias[alias] = i
		query.WriteString(alias + ":repository(owner:" + strconv.Quote(target.owner) + ",name:" + strconv.Quote(target.name) + "){pullRequests(first:100,states:OPEN,baseRefName:" + strconv.Quote(target.base) + "){nodes{" + prFields + "}}}")
	}
	query.WriteByte('}')
	return query.String(), byAlias
}

func buildUpstreamQuery(targets []upstreamQueryTarget) (string, map[string]int) {
	var query strings.Builder
	query.WriteString("query{")
	byAlias := make(map[string]int, len(targets))
	for i, target := range targets {
		alias := "r" + strconv.Itoa(i)
		byAlias[alias] = i
		query.WriteString(alias + ":repository(owner:" + strconv.Quote(target.owner) + ",name:" + strconv.Quote(target.name) + "){pullRequests(first:100,states:OPEN,headRefName:" + strconv.Quote(target.head) + "){nodes{" + prFields + "}}}")
	}
	query.WriteByte('}')
	return query.String(), byAlias
}

func isUpstreamParent(parent, child *graph.PullRequest) bool {
	return parent.HeadRepositoryID == child.RepositoryID && parent.HeadRefName == child.BaseRefName
}

func (c *Client) LoadIncluded(ctx context.Context, prs []*graph.PullRequest, progress func(int, int, string)) (updates []graph.IncludedUpdate, resultErr error) {
	updates = []graph.IncludedUpdate{}
	report := func(current, total int, phase string) {
		if progress != nil {
			progress(current, total, phase)
		}
	}
	ctx, phaseSpan := c.startSpan(ctx, "hydrate included pull requests", oteltrace.SpanInternal, oteltrace.Attributes{"pr.count": len(prs)})
	if phaseSpan != nil {
		defer func() { phaseSpan.End(resultErr, oteltrace.Attributes{"pr.update_count": len(updates)}) }()
	}
	if len(prs) == 0 {
		report(1, 1, "Fetching included pull requests")
		return updates, nil
	}
	parentsByCandidate := map[includedCandidate][]string{}
	for _, pr := range prs {
		for _, included := range pr.IncludedPRs {
			key := includedCandidate{repository: pr.Repository, number: included.Number}
			parentsByCandidate[key] = append(parentsByCandidate[key], pr.ID)
		}
	}
	if len(parentsByCandidate) == 0 {
		report(1, 1, "Fetching included pull requests")
		return updates, nil
	}
	report(0, 1, "Fetching included pull requests")
	included, err := c.fetchIncludedPullRequests(ctx, parentsByCandidate)
	if err != nil {
		return nil, err
	}
	report(1, 1, "Fetching included pull requests")
	for _, parent := range prs {
		if len(parent.IncludedPRs) == 0 {
			continue
		}
		detailsByNumber := map[int]graph.IncludedPullRequest{}
		for _, detail := range included[parent.ID] {
			detailsByNumber[detail.Number] = detail
		}
		hydrated := make([]graph.IncludedPullRequest, 0, len(parent.IncludedPRs))
		for _, candidate := range parent.IncludedPRs {
			if detail, ok := detailsByNumber[candidate.Number]; ok {
				hydrated = append(hydrated, detail)
			} else {
				hydrated = append(hydrated, candidate)
			}
		}
		updates = append(updates, graph.IncludedUpdate{PullRequestID: parent.ID, IncludedPullRequests: hydrated})
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].PullRequestID < updates[j].PullRequestID })
	return updates, nil
}

func (c *Client) InspectPullRequest(ctx context.Context, pr *graph.PullRequest) (graph.IncludedUpdate, error) {
	candidates, err := c.discoverIncludedCandidates(ctx, []*graph.PullRequest{pr}, nil)
	if err != nil {
		return graph.IncludedUpdate{}, err
	}
	included := candidates[pr.ID]
	if included == nil {
		included = []graph.IncludedPullRequest{}
	}
	return graph.IncludedUpdate{PullRequestID: pr.ID, IncludedPullRequests: included}, nil
}

func (c *Client) discoverIncludedCandidates(ctx context.Context, prs []*graph.PullRequest, progress func(int, int, string)) (map[string][]graph.IncludedPullRequest, error) {
	candidates := map[string][]graph.IncludedPullRequest{}
	if len(prs) == 0 {
		if progress != nil {
			progress(1, 1, "Inspecting included pull requests")
		}
		return candidates, nil
	}
	type job struct{ pr *graph.PullRequest }
	type scanResult struct {
		pr      *graph.PullRequest
		numbers []int
		err     error
	}
	jobs := make(chan job)
	results := make(chan scanResult, len(prs))
	var wg sync.WaitGroup
	workers := maxConcurrentGitHubRequests
	if len(prs) < workers {
		workers = len(prs)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				numbers, err := c.includedNumbersFromMessages(ctx, item.pr)
				results <- scanResult{pr: item.pr, numbers: numbers, err: err}
			}
		}()
	}
	go func() {
		for _, pr := range prs {
			jobs <- job{pr: pr}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	errorsSeen := 0
	completed := 0
	for result := range results {
		completed++
		if result.err != nil {
			errorsSeen++
		} else {
			for _, number := range result.numbers {
				candidates[result.pr.ID] = append(candidates[result.pr.ID], graph.IncludedPullRequest{Number: number})
			}
		}
		if progress != nil {
			progress(completed, len(prs), "Inspecting included pull requests")
		}
	}
	if errorsSeen > 0 {
		return nil, fmt.Errorf("%d of %d commit scans failed", errorsSeen, len(prs))
	}
	for parentID := range candidates {
		sort.Slice(candidates[parentID], func(i, j int) bool { return candidates[parentID][i].Number < candidates[parentID][j].Number })
	}
	return candidates, nil
}

func (c *Client) fetchIncludedPullRequests(ctx context.Context, parentsByCandidate map[includedCandidate][]string) (map[string][]graph.IncludedPullRequest, error) {
	query, byAlias := buildIncludedPullRequestsQuery(parentsByCandidate)
	if len(byAlias) == 0 {
		return nil, nil
	}
	var response batchPullRequestResponse
	if err := c.graphql(ctx, "batch get included pull requests", query, nil, &response); err != nil {
		return nil, err
	}
	result := map[string][]graph.IncludedPullRequest{}
	for alias, candidate := range byAlias {
		var repository struct {
			PullRequest *rawIncludedPR `json:"pr"`
		}
		if raw := response.Data[alias]; len(raw) == 0 || string(raw) == "null" {
			continue
		} else if err := json.Unmarshal(raw, &repository); err != nil {
			return nil, fmt.Errorf("decode included pull request: %w", err)
		}
		pr := repository.PullRequest
		if pr == nil {
			continue
		}
		included := graph.IncludedPullRequest{ID: pr.ID, Number: pr.Number, Title: pr.Title, URL: pr.URL, MergedAt: pr.MergedAt}
		if pr.Author != nil {
			included.Author = graph.User{Login: pr.Author.Login, AvatarURL: pr.Author.AvatarURL}
		}
		for _, parentID := range parentsByCandidate[candidate] {
			result[parentID] = append(result[parentID], included)
		}
	}
	return result, nil
}

func buildIncludedPullRequestsQuery(parentsByCandidate map[includedCandidate][]string) (string, map[string]includedCandidate) {
	candidates := make([]includedCandidate, 0, len(parentsByCandidate))
	for candidate := range parentsByCandidate {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].repository != candidates[j].repository {
			return candidates[i].repository < candidates[j].repository
		}
		return candidates[i].number < candidates[j].number
	})
	var query strings.Builder
	query.WriteString("query{")
	byAlias := make(map[string]includedCandidate, len(candidates))
	for i, candidate := range candidates {
		parts := strings.SplitN(candidate.repository, "/", 2)
		if len(parts) != 2 {
			continue
		}
		alias := "r" + strconv.Itoa(i)
		byAlias[alias] = candidate
		fmt.Fprintf(&query, "%s:repository(owner:%s,name:%s){pr:pullRequest(number:%d){id number title url mergedAt author{login avatarUrl}}}", alias, strconv.Quote(parts[0]), strconv.Quote(parts[1]), candidate.number)
	}
	query.WriteByte('}')
	return query.String(), byAlias
}

func (c *Client) includedNumbersFromMessages(ctx context.Context, pr *graph.PullRequest) (result []int, resultErr error) {
	ctx, inspectSpan := c.startSpan(ctx, "inspect pull request commits", oteltrace.SpanInternal, oteltrace.Attributes{"pr.repository": pr.Repository, "pr.number": pr.Number})
	if inspectSpan != nil {
		defer func() { inspectSpan.End(resultErr, oteltrace.Attributes{"pr.candidate_count": len(result)}) }()
	}
	numbers := map[int]bool{}
	after := ""
	for page := 0; page < 3; page++ {
		variables := map[string]string{"id": pr.ID}
		if after != "" {
			variables["after"] = after
		}
		var response commitMessagesResponse
		if err := c.graphql(ctx, "list pull request commits", commitMessagesQuery, variables, &response); err != nil {
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
	result = make([]int, 0, len(numbers))
	for number := range numbers {
		result = append(result, number)
	}
	sort.Ints(result)
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
	pr := &graph.PullRequest{ID: raw.ID, Number: raw.Number, Title: raw.Title, URL: raw.URL, IsDraft: raw.IsDraft, UpdatedAt: raw.UpdatedAt, BaseRefName: raw.BaseRefName, HeadRefName: raw.HeadRefName, BaseCommitSHA: raw.BaseRefOid, HeadCommitSHA: raw.HeadRefOid, ReviewDecision: raw.ReviewDecision, Mergeable: raw.Mergeable}
	if raw.Author != nil {
		pr.Author = graph.User{Login: raw.Author.Login, AvatarURL: raw.Author.AvatarURL}
		pr.IsBot = raw.Author.Typename == "Bot" || strings.HasSuffix(strings.ToLower(raw.Author.Login), "[bot]")
	}
	pr.RepositoryID, pr.Repository = raw.Repository.ID, raw.Repository.NameWithOwner
	pr.RepositoryURL = raw.Repository.URL
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

func (c *Client) graphql(ctx context.Context, operation, query string, variables map[string]string, target any) (resultErr error) {
	args := []string{"api", "graphql"}
	if c.Hostname != "" {
		args = append(args, "--hostname", c.Hostname)
	}
	args = append(args, "-f", "query="+query)
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := variables[key]
		args = append(args, "-F", key+"="+value)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, commandSpan := c.startSpan(ctx, "gh api graphql: "+operation, oteltrace.SpanClient, oteltrace.Attributes{
		"process.executable.name": "gh",
		"process.command_args":    append([]string{"gh"}, args...),
		"graphql.operation.name":  operation,
	})
	out, err := cmd.Output()
	if commandSpan != nil {
		processID := 0
		if cmd.Process != nil {
			processID = cmd.Process.Pid
		}
		exitCode := 0
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else if err != nil {
			exitCode = -1
		}
		commandSpan.End(err, oteltrace.Attributes{"process.pid": processID, "process.exit.code": exitCode})
	}
	if err != nil {
		return fmt.Errorf("GitHub API: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func (c *Client) startSpan(ctx context.Context, name string, kind oteltrace.SpanKind, attributes oteltrace.Attributes) (context.Context, oteltrace.Span) {
	if c.Tracer == nil {
		return ctx, nil
	}
	return c.Tracer.Start(ctx, name, kind, attributes)
}
