package domain

import "context"

type Permissions struct {
	AutoApproveMovies  bool `json:"autoApproveMovies"`
	AutoApproveTV      bool `json:"autoApproveTv"`
	ViewOthersRequests bool `json:"viewOthersRequests"`
	ReportIssues       bool `json:"reportIssues"`
	ManageFiles        bool `json:"manageFiles"`
}

// Principal is the signed-in user. Identity's middleware sets it and every controller reads it.
type Principal struct {
	User        UserID      `json:"user"`
	Admin       bool        `json:"admin"`
	Permissions Permissions `json:"permissions"`
}

type principalKey struct{}

// WithPrincipal returns ctx carrying p. Only Identity's middleware and tests call it.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
