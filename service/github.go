package service

import (
	"context"
	"errors"
	"fmt"

	gh "github.com/hollis-labs/folio/internal/github"
)

// PublishOptions parameterises Service.PublishToGitHub. TargetDir must
// already contain a rendered project; Owner + Repo + Visibility are
// required. Branch defaults to "main"; Push defaults to true (set false
// for flows that want operator approval before pushing).
type PublishOptions struct {
	TargetDir   string
	Owner       string
	Repo        string
	Visibility  string
	Description string
	Branch      string
	Push        bool
	// Runner lets callers (mainly tests) inject a fake exec surface. When
	// nil, gh.ExecRunner is used.
	Runner gh.Runner
}

// PublishResult summarizes a successful PublishToGitHub call.
type PublishResult struct {
	URL    string
	Owner  string
	Repo   string
	Branch string
	Pushed bool
}

// PublishToGitHub runs the gh+git publish flow against an already-rendered
// project at opts.TargetDir. Preflight (gh / git binary checks + gh auth
// status) runs first; on failure the local tree is untouched. The caller
// is responsible for the render that produced TargetDir.
func (s *Service) PublishToGitHub(ctx context.Context, opts PublishOptions) (PublishResult, error) {
	if err := validatePublishOpts(opts); err != nil {
		return PublishResult{}, err
	}
	runner := opts.Runner
	if runner == nil {
		runner = gh.ExecRunner{}
	}
	if err := gh.Preflight(ctx, runner); err != nil {
		return PublishResult{}, translateGHError(err)
	}
	res, err := gh.CreateRepo(ctx, runner, gh.CreateOpts{
		TargetDir:   opts.TargetDir,
		Owner:       opts.Owner,
		Repo:        opts.Repo,
		Visibility:  gh.Visibility(opts.Visibility),
		Description: opts.Description,
		Branch:      opts.Branch,
		Push:        opts.Push,
	})
	if err != nil {
		return PublishResult{}, translateGHError(err)
	}
	return PublishResult{
		URL:    res.URL,
		Owner:  res.Owner,
		Repo:   res.Repo,
		Branch: res.Branch,
		Pushed: res.Pushed,
	}, nil
}

// PublishErrorCode reports the ErrorCode of a publish-pipeline error, or
// "" if err is not a *service.Error wrapping one of the publish codes.
// Useful for the CLI when classifying the failure for retry hints.
func PublishErrorCode(err error) ErrorCode {
	var sErr *Error
	if errors.As(err, &sErr) {
		return sErr.Code
	}
	return ""
}

func validatePublishOpts(opts PublishOptions) error {
	if opts.TargetDir == "" {
		return newErr(ErrInputInvalid, "publish: target dir is empty", nil)
	}
	if opts.Owner == "" {
		return newErr(ErrInputMissing, "publish: --github-owner not set and no inputs.github_owner present", nil)
	}
	if opts.Repo == "" {
		return newErr(ErrInputMissing, "publish: repo name is empty (defaults to basename(target))", nil)
	}
	if opts.Visibility == "" {
		return newErr(ErrInputInvalid, "publish: visibility is empty", nil)
	}
	if !gh.Visibility(opts.Visibility).Valid() {
		return newErr(ErrInputInvalid, fmt.Sprintf("publish: invalid visibility %q (want public|private|internal)", opts.Visibility), nil)
	}
	return nil
}

// translateGHError lifts an internal/github *Error into a service *Error
// so the CLI sees a uniform error type. The internal code is preserved
// as-is in the service code field — internal/github codes (gh_missing,
// gh_unauthenticated, git_missing, repo_exists, publish_failed) all live
// in the same ErrorCode namespace.
func translateGHError(err error) error {
	if err == nil {
		return nil
	}
	var ghErr *gh.Error
	if errors.As(err, &ghErr) {
		return &Error{
			Code:    ErrorCode(ghErr.Code),
			Message: fmt.Sprintf("publish %s: %s", ghErr.Op, ghErr.Output),
			Err:     ghErr.Err,
		}
	}
	return newErr(ErrInternal, "publish: unexpected error", err)
}
