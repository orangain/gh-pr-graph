package github

import "testing"

func TestConvertReviewSummary(t *testing.T) {
	raw := rawPR{ID: "pr1"}
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
}
