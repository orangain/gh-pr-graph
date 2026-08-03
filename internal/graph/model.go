package graph

import "time"

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

type SearchOptions struct {
	Query           string
	Authored        bool
	Assigned        bool
	ReviewRequested bool
}

type PullRequest struct {
	ID                string                `json:"id"`
	Number            int                   `json:"number"`
	Title             string                `json:"title"`
	URL               string                `json:"url"`
	IsDraft           bool                  `json:"isDraft"`
	UpdatedAt         time.Time             `json:"updatedAt"`
	Author            User                  `json:"author"`
	IsBot             bool                  `json:"isBot,omitempty"`
	RepositoryID      string                `json:"repositoryId"`
	Repository        string                `json:"repository"`
	RepositoryURL     string                `json:"repositoryUrl"`
	DefaultBranch     string                `json:"defaultBranch"`
	BaseRefName       string                `json:"baseRefName"`
	HeadRefName       string                `json:"headRefName"`
	BaseCommitSHA     string                `json:"baseCommitSha"`
	HeadCommitSHA     string                `json:"headCommitSha"`
	HeadRepositoryID  string                `json:"headRepositoryId"`
	HeadRepository    string                `json:"headRepository"`
	Assignees         []User                `json:"assignees"`
	ReviewDecision    string                `json:"reviewDecision,omitempty"`
	ReviewApproved    int                   `json:"reviewApproved"`
	ReviewTotal       int                   `json:"reviewTotal"`
	TeamReviewPending bool                  `json:"teamReviewPending,omitempty"`
	ReReviewRequested bool                  `json:"reReviewRequested,omitempty"`
	CIState           string                `json:"ciState,omitempty"`
	Mergeable         string                `json:"mergeable,omitempty"`
	Relation          string                `json:"relation"`
	Source            string                `json:"source"`
	IncludedPRs       []IncludedPullRequest `json:"includedPullRequests,omitempty"`
	IncludedTruncated bool                  `json:"includedPullRequestsTruncated,omitempty"`
}

type Node struct {
	ID     string       `json:"id"`
	Kind   string       `json:"kind"`
	Rank   int          `json:"rank"`
	Repo   *Repository  `json:"repository,omitempty"`
	Branch *Branch      `json:"branch,omitempty"`
	PR     *PullRequest `json:"pullRequest,omitempty"`
}

type Branch struct {
	RepositoryID string `json:"repositoryId"`
	Name         string `json:"name"`
	URL          string `json:"url,omitempty"`
}

type Repository struct {
	ID            string `json:"id"`
	NameWithOwner string `json:"nameWithOwner"`
	URL           string `json:"url"`
	DefaultBranch string `json:"defaultBranch"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Dashed bool   `json:"dashed,omitempty"`
}

type Result struct {
	Nodes     []Node    `json:"nodes"`
	Edges     []Edge    `json:"edges"`
	Warnings  []string  `json:"warnings"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type IncludedPullRequest struct {
	ID       string     `json:"id"`
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	URL      string     `json:"url"`
	Author   User       `json:"author"`
	MergedAt *time.Time `json:"mergedAt,omitempty"`
}

type IncludedUpdate struct {
	PullRequestID        string                `json:"pullRequestId"`
	IncludedPullRequests []IncludedPullRequest `json:"includedPullRequests"`
	Truncated            bool                  `json:"includedPullRequestsTruncated,omitempty"`
}

func RelationFor(pr *PullRequest, viewer string, directlyReviewRequested bool) string {
	if pr.Author.Login == viewer {
		return "mine"
	}
	for _, assignee := range pr.Assignees {
		if assignee.Login == viewer {
			return "assigned"
		}
	}
	if directlyReviewRequested {
		return "review-requested"
	}
	return "other"
}
