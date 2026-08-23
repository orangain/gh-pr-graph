package graph

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

func Build(prs []*PullRequest, warnings []string) Result {
	result := Result{
		Nodes:     make([]Node, 0, len(prs)),
		Edges:     make([]Edge, 0, len(prs)),
		Warnings:  warnings,
		UpdatedAt: time.Now().UTC(),
	}
	byID := make(map[string]*PullRequest, len(prs))
	head := make(map[string][]*PullRequest)
	repos := make(map[string]*Repository)

	for _, pr := range prs {
		byID[pr.ID] = pr
		key := refKey(pr.HeadRepositoryID, pr.HeadRefName)
		head[key] = append(head[key], pr)
		repo := repos[pr.RepositoryID]
		if repo == nil {
			repo = &Repository{ID: pr.RepositoryID, NameWithOwner: pr.Repository, DefaultBranch: pr.DefaultBranch}
			repos[pr.RepositoryID] = repo
		}
		if pr.RepositoryURL != "" {
			repo.URL = pr.RepositoryURL
		}
	}

	ranks := make(map[string]int, len(prs))
	branches := make(map[string]*Branch)
	for _, pr := range prs {
		parents := head[refKey(pr.RepositoryID, pr.BaseRefName)]
		if !hasDifferentPR(parents, pr.ID) && pr.BaseRefName != pr.DefaultBranch {
			ranks[pr.ID] = 2
			key := refKey(pr.RepositoryID, pr.BaseRefName)
			branch := branches[key]
			if branch == nil {
				branch = &Branch{RepositoryID: pr.RepositoryID, Name: pr.BaseRefName}
				branches[key] = branch
			}
			if branch.URL == "" {
				branch.URL = branchURL(pr.RepositoryURL, pr.BaseRefName)
			}
		} else {
			ranks[pr.ID] = 1
		}
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
	branchList := make([]*Branch, 0, len(branches))
	for _, branch := range branches {
		branchList = append(branchList, branch)
	}
	sort.Slice(branchList, func(i, j int) bool {
		if branchList[i].RepositoryID != branchList[j].RepositoryID {
			return branchList[i].RepositoryID < branchList[j].RepositoryID
		}
		return branchList[i].Name < branchList[j].Name
	})
	for _, branch := range branchList {
		id := branchNodeID(branch.RepositoryID, branch.Name)
		result.Nodes = append(result.Nodes, Node{ID: id, Kind: "branch", Rank: 1, Branch: branch})
		result.Edges = append(result.Edges, Edge{ID: "root:" + id, Source: "repo:" + branch.RepositoryID, Target: id, Dashed: true})
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
		if !hasDifferentPR(parents, pr.ID) {
			if pr.BaseRefName != pr.DefaultBranch {
				branchID := branchNodeID(pr.RepositoryID, pr.BaseRefName)
				result.Edges = append(result.Edges, Edge{ID: fmt.Sprintf("%s:%s", branchID, pr.ID), Source: branchID, Target: "pr:" + pr.ID})
			} else {
				result.Edges = append(result.Edges, Edge{ID: "root:" + pr.ID, Source: "repo:" + pr.RepositoryID, Target: "pr:" + pr.ID})
			}
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

func branchNodeID(repoID, ref string) string { return "branch:" + repoID + ":" + ref }

func branchURL(repositoryURL, ref string) string {
	if repositoryURL == "" || ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.TrimRight(repositoryURL, "/") + "/tree/" + strings.Join(parts, "/")
}

func hasDifferentPR(prs []*PullRequest, id string) bool {
	for _, pr := range prs {
		if pr.ID != id {
			return true
		}
	}
	return false
}
