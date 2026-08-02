package graph

import (
	"fmt"
	"sort"
	"time"
)

func Build(prs []*PullRequest, warnings []string) Result {
	result := Result{Warnings: warnings, UpdatedAt: time.Now().UTC()}
	byID := make(map[string]*PullRequest, len(prs))
	head := make(map[string][]*PullRequest)
	repos := make(map[string]*Repository)

	for _, pr := range prs {
		byID[pr.ID] = pr
		key := refKey(pr.HeadRepositoryID, pr.HeadRefName)
		head[key] = append(head[key], pr)
		repos[pr.RepositoryID] = &Repository{ID: pr.RepositoryID, NameWithOwner: pr.Repository, DefaultBranch: pr.DefaultBranch}
	}

	ranks := make(map[string]int, len(prs))
	for _, pr := range prs {
		ranks[pr.ID] = 1
	}
	for pass := 0; pass < len(prs); pass++ {
		changed := false
		for _, child := range prs {
			for _, parent := range head[refKey(child.RepositoryID, child.BaseRefName)] {
				if parent.ID == child.ID {
					continue
				}
				if next := ranks[parent.ID] + 1; next > ranks[child.ID] && next <= len(prs)+1 {
					ranks[child.ID], changed = next, true
				}
			}
		}
		if !changed {
			break
		}
	}

	repoList := make([]*Repository, 0, len(repos))
	for _, repo := range repos {
		repoList = append(repoList, repo)
	}
	sort.Slice(repoList, func(i, j int) bool { return repoList[i].NameWithOwner < repoList[j].NameWithOwner })
	for _, repo := range repoList {
		result.Nodes = append(result.Nodes, Node{ID: "repo:" + repo.ID, Kind: "repository", Rank: 0, Repo: repo})
	}

	sort.Slice(prs, func(i, j int) bool {
		if ranks[prs[i].ID] != ranks[prs[j].ID] {
			return ranks[prs[i].ID] < ranks[prs[j].ID]
		}
		if prs[i].Repository != prs[j].Repository {
			return prs[i].Repository < prs[j].Repository
		}
		return prs[i].Number < prs[j].Number
	})
	for _, pr := range prs {
		result.Nodes = append(result.Nodes, Node{ID: "pr:" + pr.ID, Kind: "pullRequest", Rank: ranks[pr.ID], PR: pr})
		parents := head[refKey(pr.RepositoryID, pr.BaseRefName)]
		if len(parents) == 0 && pr.BaseRefName == pr.DefaultBranch {
			result.Edges = append(result.Edges, Edge{ID: "root:" + pr.ID, Source: "repo:" + pr.RepositoryID, Target: "pr:" + pr.ID})
		}
		for _, parent := range parents {
			if parent.ID != pr.ID {
				result.Edges = append(result.Edges, Edge{ID: fmt.Sprintf("%s:%s", parent.ID, pr.ID), Source: "pr:" + parent.ID, Target: "pr:" + pr.ID})
			}
		}
	}
	return result
}

func refKey(repoID, ref string) string { return repoID + "\x00" + ref }
