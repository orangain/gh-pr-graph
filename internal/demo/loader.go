package demo

import (
	"context"
	"strings"
	"time"

	"github.com/orangain/gh-pr-graph/internal/graph"
)

type Loader struct{}

func New() *Loader { return &Loader{} }

func (l *Loader) Load(ctx context.Context, options graph.SearchOptions) (graph.Result, error) {
	return l.LoadProgress(ctx, options, nil)
}

func (l *Loader) LoadProgress(_ context.Context, options graph.SearchOptions, progress func(int, int, string)) (graph.Result, error) {
	if progress != nil {
		progress(1, 1, "Searching pull requests")
		progress(1, 1, "Discovering stacked pull requests")
	}
	all := pullRequests()
	selected := map[string]bool{}
	if options.Authored {
		selected["authored"] = true
	}
	if options.Assigned {
		selected["assigned-bot"] = true
	}
	if options.ReviewRequested {
		selected["review-root"] = true
		selected["review-child"] = true
		selected["other-child"] = true
	}
	if strings.TrimSpace(options.Query) != "" {
		for _, pr := range all {
			selected[pr.ID] = true
		}
	}
	prs := make([]*graph.PullRequest, 0, len(selected))
	for _, pr := range all {
		if selected[pr.ID] {
			copy := *pr
			prs = append(prs, &copy)
		}
	}
	return graph.Build(prs, nil), nil
}

func (l *Loader) InspectPullRequest(_ context.Context, pr *graph.PullRequest) (graph.IncludedUpdate, error) {
	included := []graph.IncludedPullRequest{}
	if pr.ID == "authored" {
		included = append(included, graph.IncludedPullRequest{Number: 88})
	}
	return graph.IncludedUpdate{PullRequestID: pr.ID, IncludedPullRequests: included}, nil
}

func (l *Loader) LoadIncluded(_ context.Context, prs []*graph.PullRequest, progress func(int, int, string)) ([]graph.IncludedUpdate, error) {
	updates := []graph.IncludedUpdate{}
	for _, pr := range prs {
		included := pr.IncludedPRs
		if pr.ID == "authored" && len(included) > 0 {
			mergedAt := time.Date(2026, time.July, 30, 9, 30, 0, 0, time.UTC)
			included = []graph.IncludedPullRequest{{ID: "included-88", Number: 88, Title: "Add reusable authentication primitives", URL: "https://github.com/acme/atlas/pull/88", Author: graph.User{Login: "orangain"}, MergedAt: &mergedAt}}
		}
		updates = append(updates, graph.IncludedUpdate{PullRequestID: pr.ID, IncludedPullRequests: included})
	}
	if progress != nil {
		progress(1, 1, "Fetching included pull requests")
	}
	return updates, nil
}

func pullRequests() []*graph.PullRequest {
	updated := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	user := graph.User{Login: "orangain", AvatarURL: "https://github.com/orangain.png?size=40"}
	prs := []*graph.PullRequest{
		{ID: "authored", Number: 104, Title: "Ship the new command palette", URL: "https://github.com/acme/atlas/pull/104", UpdatedAt: updated, Author: user, RepositoryID: "atlas", Repository: "acme/atlas", RepositoryURL: "https://github.com/acme/atlas", DefaultBranch: "main", BaseRefName: "main", HeadRefName: "command-palette", HeadRepositoryID: "atlas", HeadRepository: "acme/atlas", ReviewDecision: "APPROVED", ReviewApproved: 2, ReviewTotal: 2, CIState: "SUCCESS", Mergeable: "MERGEABLE", Relation: "mine", Source: "search"},
		{ID: "review-root", Number: 217, Title: "Introduce the agent workflow engine", URL: "https://github.com/acme/atlas/pull/217", UpdatedAt: updated, Author: graph.User{Login: "maya", AvatarURL: "https://github.com/identicons/maya.png"}, RepositoryID: "atlas", Repository: "acme/atlas", RepositoryURL: "https://github.com/acme/atlas", DefaultBranch: "main", BaseRefName: "main", HeadRefName: "agent-workflows", HeadRepositoryID: "atlas", HeadRepository: "acme/atlas", ReviewApproved: 1, ReviewTotal: 3, ReReviewRequested: true, CIState: "SUCCESS", Mergeable: "MERGEABLE", Relation: "review-requested", Source: "search"},
		{ID: "review-child", Number: 221, Title: "Add parallel tool execution", URL: "https://github.com/acme/atlas/pull/221", UpdatedAt: updated, Author: graph.User{Login: "leo", AvatarURL: "https://github.com/identicons/leo.png"}, RepositoryID: "atlas", Repository: "acme/atlas", RepositoryURL: "https://github.com/acme/atlas", DefaultBranch: "main", BaseRefName: "agent-workflows", HeadRefName: "parallel-tools", HeadRepositoryID: "atlas", HeadRepository: "acme/atlas", ReviewApproved: 0, ReviewTotal: 2, CIState: "PENDING", Mergeable: "MERGEABLE", Relation: "review-requested", Source: "downstream"},
		{ID: "other-child", Number: 223, Title: "Persist agent execution history", URL: "https://github.com/acme/atlas/pull/223", IsDraft: true, UpdatedAt: updated, Author: graph.User{Login: "nora", AvatarURL: "https://github.com/identicons/nora.png"}, RepositoryID: "atlas", Repository: "acme/atlas", RepositoryURL: "https://github.com/acme/atlas", DefaultBranch: "main", BaseRefName: "agent-workflows", HeadRefName: "execution-history", HeadRepositoryID: "atlas", HeadRepository: "acme/atlas", ReviewApproved: 0, ReviewTotal: 1, CIState: "FAILURE", Mergeable: "CONFLICTING", Relation: "other", Source: "downstream"},
		{ID: "assigned-bot", Number: 73, Title: "Bump OpenTelemetry dependencies", URL: "https://github.com/acme/beacon/pull/73", UpdatedAt: updated, Author: graph.User{Login: "dependabot[bot]", AvatarURL: "https://github.com/dependabot.png?size=40"}, IsBot: true, RepositoryID: "beacon", Repository: "acme/beacon", RepositoryURL: "https://github.com/acme/beacon", DefaultBranch: "main", BaseRefName: "main", HeadRefName: "dependabot/go-modules/otel", HeadRepositoryID: "beacon", HeadRepository: "acme/beacon", Assignees: []graph.User{user}, ReviewApproved: 0, ReviewTotal: 1, CIState: "SUCCESS", Mergeable: "MERGEABLE", Relation: "assigned", Source: "search"},
	}
	for _, pr := range prs {
		pr.BaseCommitSHA = "demo-base-" + pr.ID
		pr.HeadCommitSHA = "demo-head-" + pr.ID
	}
	return prs
}
