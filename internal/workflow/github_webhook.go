package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

const DefaultBranchNamePattern = `^(feature|feat|fix|bugfix|hotfix|chore|docs|refactor|test|ci|build|release|dependabot)/[a-z0-9._-]+$`

type GitHubWebhookEvent struct {
	Event      string          `json:"event"`
	DeliveryID string          `json:"deliveryId"`
	Payload    json.RawMessage `json:"-"`
}

type GitHubWebhookResult struct {
	Event      string   `json:"event"`
	DeliveryID string   `json:"deliveryId,omitempty"`
	Actions    []string `json:"actions"`
}

type GitHubAPI interface {
	AddLabels(ctx context.Context, owner, repo string, issueNumber int, labels []string) error
	CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) error
	CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error
}

type SNSPublisher interface {
	Publish(context.Context, *sns.PublishInput, ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type CommitStatus struct {
	State       string `json:"state"`
	Context     string `json:"context"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url,omitempty"`
}

type Options struct {
	GitHub                    GitHubAPI
	SNS                       SNSPublisher
	Logger                    *slog.Logger
	BranchNamePattern         string
	AutoLabelPullRequests     bool
	DeploymentSummaryTopicARN string
}

type GitHubWebhookProcessor struct {
	github                    GitHubAPI
	sns                       SNSPublisher
	logger                    *slog.Logger
	branchNamePattern         *regexp.Regexp
	autoLabelPullRequests     bool
	deploymentSummaryTopicARN string
}

func NewGitHubWebhookProcessor(options Options) (*GitHubWebhookProcessor, error) {
	pattern := strings.TrimSpace(options.BranchNamePattern)
	if pattern == "" {
		pattern = DefaultBranchNamePattern
	}

	compiledPattern, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile branch name pattern: %w", err)
	}

	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	return &GitHubWebhookProcessor{
		github:                    options.GitHub,
		sns:                       options.SNS,
		logger:                    logger,
		branchNamePattern:         compiledPattern,
		autoLabelPullRequests:     options.AutoLabelPullRequests,
		deploymentSummaryTopicARN: strings.TrimSpace(options.DeploymentSummaryTopicARN),
	}, nil
}

func (p *GitHubWebhookProcessor) ProcessGitHubWebhook(ctx context.Context, event GitHubWebhookEvent) (GitHubWebhookResult, error) {
	result := GitHubWebhookResult{
		Event:      event.Event,
		DeliveryID: event.DeliveryID,
		Actions:    []string{},
	}

	switch event.Event {
	case "ping":
		result.Actions = append(result.Actions, "ping_acknowledged")
	case "pull_request":
		actions, err := p.processPullRequest(ctx, event.Payload)
		if err != nil {
			return result, err
		}
		result.Actions = append(result.Actions, actions...)
	case "deployment_status":
		actions, err := p.processDeploymentStatus(ctx, event)
		if err != nil {
			return result, err
		}
		result.Actions = append(result.Actions, actions...)
	default:
		result.Actions = append(result.Actions, "event_ignored")
	}

	return result, nil
}

func (p *GitHubWebhookProcessor) processPullRequest(ctx context.Context, payload json.RawMessage) ([]string, error) {
	var event pullRequestPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("decode pull_request payload: %w", err)
	}

	owner, repo, ok := ownerRepo(event.Repository.FullName)
	if !ok {
		return nil, fmt.Errorf("pull_request payload missing repository full_name")
	}

	actions := []string{}
	branch := event.PullRequest.Head.Ref
	sha := event.PullRequest.Head.SHA
	labels := labelsForPullRequest(branch, event.PullRequest.Title)

	if p.autoLabelPullRequests && len(labels) > 0 {
		if p.github == nil {
			actions = append(actions, "labels_skipped_missing_github_token")
		} else if err := p.github.AddLabels(ctx, owner, repo, event.PullRequest.Number, labels); err != nil {
			p.logger.Warn("failed to add pull request labels", "error", err, "repo", event.Repository.FullName, "number", event.PullRequest.Number)
			actions = append(actions, "labels_failed")
		} else {
			actions = append(actions, "labels_added")
		}
	}

	if branch == "" || sha == "" {
		actions = append(actions, "branch_check_skipped_missing_head")
		return actions, nil
	}

	if p.branchNamePattern.MatchString(branch) {
		actions = append(actions, p.createBranchStatus(ctx, owner, repo, sha, CommitStatus{
			State:       "success",
			Context:     "platform/branch-name",
			Description: "Branch name follows the platform convention.",
		}))
		return actions, nil
	}

	actions = append(actions, p.createBranchStatus(ctx, owner, repo, sha, CommitStatus{
		State:       "failure",
		Context:     "platform/branch-name",
		Description: "Branch name must follow the platform convention.",
	}))

	if p.github == nil {
		actions = append(actions, "branch_comment_skipped_missing_github_token")
		return actions, nil
	}

	comment := fmt.Sprintf("Branch `%s` does not match the required naming convention `%s`.", branch, p.branchNamePattern.String())
	if err := p.github.CreateIssueComment(ctx, owner, repo, event.PullRequest.Number, comment); err != nil {
		p.logger.Warn("failed to comment on pull request branch naming violation", "error", err, "repo", event.Repository.FullName, "number", event.PullRequest.Number)
		return append(actions, "branch_comment_failed"), nil
	}

	return append(actions, "branch_comment_posted"), nil
}

func (p *GitHubWebhookProcessor) createBranchStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) string {
	if p.github == nil {
		return "branch_status_skipped_missing_github_token"
	}
	if err := p.github.CreateCommitStatus(ctx, owner, repo, sha, status); err != nil {
		p.logger.Warn("failed to create branch naming status", "error", err, "repo", owner+"/"+repo, "sha", sha)
		return "branch_status_failed"
	}
	return "branch_status_created"
}

func (p *GitHubWebhookProcessor) processDeploymentStatus(ctx context.Context, event GitHubWebhookEvent) ([]string, error) {
	var payload deploymentStatusPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode deployment_status payload: %w", err)
	}

	if p.deploymentSummaryTopicARN == "" {
		return []string{"deployment_summary_skipped_missing_topic"}, nil
	}
	if p.sns == nil {
		return []string{"deployment_summary_skipped_missing_sns_client"}, nil
	}

	message := deploymentSummary{
		Repository:     payload.Repository.FullName,
		Environment:    firstNonEmpty(payload.DeploymentStatus.Environment, payload.Deployment.Environment),
		State:          payload.DeploymentStatus.State,
		Ref:            payload.Deployment.Ref,
		SHA:            payload.Deployment.SHA,
		Description:    payload.DeploymentStatus.Description,
		TargetURL:      payload.DeploymentStatus.TargetURL,
		EnvironmentURL: payload.DeploymentStatus.EnvironmentURL,
		LogURL:         payload.DeploymentStatus.LogURL,
		DeliveryID:     event.DeliveryID,
		CreatedAt:      payload.DeploymentStatus.CreatedAt,
	}

	body, err := json.MarshalIndent(message, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode deployment summary: %w", err)
	}

	subject := fmt.Sprintf("GitHub deployment %s: %s", message.State, message.Repository)
	if len(subject) > 100 {
		subject = subject[:100]
	}

	if _, err := p.sns.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(p.deploymentSummaryTopicARN),
		Subject:  aws.String(subject),
		Message:  aws.String(string(body)),
	}); err != nil {
		p.logger.Warn("failed to publish deployment summary", "error", err, "topic_arn", p.deploymentSummaryTopicARN)
		return []string{"deployment_summary_failed"}, nil
	}

	return []string{"deployment_summary_published"}, nil
}

func labelsForPullRequest(branch, title string) []string {
	source := strings.ToLower(strings.TrimSpace(branch) + " " + strings.TrimSpace(title))
	labelSet := map[string]struct{}{}

	add := func(label string) {
		labelSet[label] = struct{}{}
	}

	switch {
	case strings.HasPrefix(source, "feature/"), strings.HasPrefix(source, "feat/"):
		add("enhancement")
	case strings.HasPrefix(source, "fix/"), strings.HasPrefix(source, "bugfix/"), strings.HasPrefix(source, "hotfix/"):
		add("bug")
	case strings.HasPrefix(source, "docs/"):
		add("documentation")
	case strings.HasPrefix(source, "dependabot/"):
		add("dependencies")
	case strings.HasPrefix(source, "release/"):
		add("release")
	case strings.HasPrefix(source, "chore/"), strings.HasPrefix(source, "ci/"), strings.HasPrefix(source, "build/"):
		add("maintenance")
	}

	if strings.Contains(source, "terraform") || strings.Contains(source, "ecs") || strings.Contains(source, "s3") || strings.Contains(source, "aws") || strings.Contains(source, "infra") {
		add("infrastructure")
	}

	labels := make([]string, 0, len(labelSet))
	for label := range labelSet {
		labels = append(labels, label)
	}
	return labels
}

type GitHubClient struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewGitHubClient(token, baseURL string, httpClient *http.Client) *GitHubClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &GitHubClient{
		token:      strings.TrimSpace(token),
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (c *GitHubClient) AddLabels(ctx context.Context, owner, repo string, issueNumber int, labels []string) error {
	for _, label := range labels {
		if err := c.createLabel(ctx, owner, repo, label); err != nil {
			var githubErr *GitHubError
			if !asGitHubError(err, &githubErr) || githubErr.StatusCode != http.StatusUnprocessableEntity {
				return err
			}
		}
	}

	payload := map[string][]string{"labels": labels}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/labels", pathEscape(owner), pathEscape(repo), issueNumber), payload)
}

func (c *GitHubClient) CreateIssueComment(ctx context.Context, owner, repo string, issueNumber int, body string) error {
	payload := map[string]string{"body": body}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", pathEscape(owner), pathEscape(repo), issueNumber), payload)
}

func (c *GitHubClient) CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/statuses/%s", pathEscape(owner), pathEscape(repo), pathEscape(sha)), status)
}

func (c *GitHubClient) createLabel(ctx context.Context, owner, repo, label string) error {
	definition := labelDefinition(label)
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/labels", pathEscape(owner), pathEscape(repo)), definition)
}

func (c *GitHubClient) do(ctx context.Context, method, path string, payload any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode github request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create github request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("github request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	errorBody, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
	return &GitHubError{
		StatusCode: response.StatusCode,
		Status:     response.Status,
		Body:       strings.TrimSpace(string(errorBody)),
	}
}

type GitHubError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *GitHubError) Error() string {
	return fmt.Sprintf("github request returned %s: %s", e.Status, e.Body)
}

func asGitHubError(err error, target **GitHubError) bool {
	if githubErr, ok := err.(*GitHubError); ok {
		*target = githubErr
		return true
	}
	return false
}

func ownerRepo(fullName string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(fullName), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func pathEscape(value string) string {
	return url.PathEscape(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func labelDefinition(label string) map[string]string {
	definitions := map[string]map[string]string{
		"bug": {
			"name":        "bug",
			"color":       "d73a4a",
			"description": "Something is not working",
		},
		"dependencies": {
			"name":        "dependencies",
			"color":       "0366d6",
			"description": "Dependency updates",
		},
		"documentation": {
			"name":        "documentation",
			"color":       "0075ca",
			"description": "Documentation changes",
		},
		"enhancement": {
			"name":        "enhancement",
			"color":       "a2eeef",
			"description": "New feature or request",
		},
		"infrastructure": {
			"name":        "infrastructure",
			"color":       "5319e7",
			"description": "Cloud, deployment, or platform infrastructure changes",
		},
		"maintenance": {
			"name":        "maintenance",
			"color":       "fbca04",
			"description": "Routine maintenance or build changes",
		},
		"release": {
			"name":        "release",
			"color":       "0e8a16",
			"description": "Release preparation",
		},
	}
	if definition, ok := definitions[label]; ok {
		return definition
	}
	return map[string]string{
		"name":        label,
		"color":       "ededed",
		"description": "Managed by platform service",
	}
}

type repositoryPayload struct {
	FullName string `json:"full_name"`
}

type pullRequestPayload struct {
	Action      string            `json:"action"`
	Repository  repositoryPayload `json:"repository"`
	PullRequest struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

type deploymentStatusPayload struct {
	Action           string            `json:"action"`
	Repository       repositoryPayload `json:"repository"`
	Deployment       deploymentPayload `json:"deployment"`
	DeploymentStatus struct {
		State          string `json:"state"`
		Environment    string `json:"environment"`
		Description    string `json:"description"`
		TargetURL      string `json:"target_url"`
		EnvironmentURL string `json:"environment_url"`
		LogURL         string `json:"log_url"`
		CreatedAt      string `json:"created_at"`
	} `json:"deployment_status"`
}

type deploymentPayload struct {
	Ref         string `json:"ref"`
	SHA         string `json:"sha"`
	Environment string `json:"environment"`
}

type deploymentSummary struct {
	Repository     string `json:"repository"`
	Environment    string `json:"environment,omitempty"`
	State          string `json:"state"`
	Ref            string `json:"ref,omitempty"`
	SHA            string `json:"sha,omitempty"`
	Description    string `json:"description,omitempty"`
	TargetURL      string `json:"targetUrl,omitempty"`
	EnvironmentURL string `json:"environmentUrl,omitempty"`
	LogURL         string `json:"logUrl,omitempty"`
	DeliveryID     string `json:"deliveryId,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
}
