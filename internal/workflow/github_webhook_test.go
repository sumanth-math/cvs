package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
)

func TestPullRequestWebhookLabelsAndPassesBranchCheck(t *testing.T) {
	github := &fakeGitHubAPI{}
	processor, err := NewGitHubWebhookProcessor(Options{
		GitHub:                github,
		AutoLabelPullRequests: true,
	})
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	payload := pullRequestTestPayload("feature/add-webhooks", "Add AWS webhook workflow")
	result, err := processor.ProcessGitHubWebhook(context.Background(), GitHubWebhookEvent{
		Event:      "pull_request",
		DeliveryID: "delivery-1",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("process webhook: %v", err)
	}

	if !contains(result.Actions, "labels_added") {
		t.Fatalf("expected labels_added action, got %#v", result.Actions)
	}
	if !contains(github.labels, "enhancement") || !contains(github.labels, "infrastructure") {
		t.Fatalf("expected enhancement and infrastructure labels, got %#v", github.labels)
	}
	if github.status.State != "success" {
		t.Fatalf("expected success status, got %#v", github.status)
	}
	if github.comment != "" {
		t.Fatalf("did not expect comment, got %q", github.comment)
	}
}

func TestPullRequestWebhookFlagsInvalidBranch(t *testing.T) {
	github := &fakeGitHubAPI{}
	processor, err := NewGitHubWebhookProcessor(Options{
		GitHub:                github,
		AutoLabelPullRequests: true,
	})
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	result, err := processor.ProcessGitHubWebhook(context.Background(), GitHubWebhookEvent{
		Event:   "pull_request",
		Payload: pullRequestTestPayload("badbranch", "Update service"),
	})
	if err != nil {
		t.Fatalf("process webhook: %v", err)
	}

	if !contains(result.Actions, "branch_comment_posted") {
		t.Fatalf("expected branch comment action, got %#v", result.Actions)
	}
	if github.status.State != "failure" {
		t.Fatalf("expected failure status, got %#v", github.status)
	}
	if !strings.Contains(github.comment, "badbranch") {
		t.Fatalf("expected comment to mention branch, got %q", github.comment)
	}
}

func TestDeploymentStatusPublishesSummary(t *testing.T) {
	publisher := &fakeSNSPublisher{}
	processor, err := NewGitHubWebhookProcessor(Options{
		SNS:                       publisher,
		DeploymentSummaryTopicARN: "arn:aws:sns:us-east-1:123456789012:deployments",
	})
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	payload := []byte(`{
		"repository":{"full_name":"sumanth-math/cvs"},
		"deployment":{"ref":"main","sha":"abc123","environment":"dev"},
		"deployment_status":{"state":"success","description":"deployed","environment_url":"https://example.com","created_at":"2026-04-30T02:34:28Z"}
	}`)
	result, err := processor.ProcessGitHubWebhook(context.Background(), GitHubWebhookEvent{
		Event:      "deployment_status",
		DeliveryID: "delivery-2",
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("process webhook: %v", err)
	}

	if !contains(result.Actions, "deployment_summary_published") {
		t.Fatalf("expected deployment summary action, got %#v", result.Actions)
	}
	if publisher.topicArn != "arn:aws:sns:us-east-1:123456789012:deployments" {
		t.Fatalf("unexpected topic arn: %s", publisher.topicArn)
	}
	if !strings.Contains(publisher.message, `"repository": "sumanth-math/cvs"`) {
		t.Fatalf("summary did not include repository: %s", publisher.message)
	}
}

type fakeGitHubAPI struct {
	labels  []string
	status  CommitStatus
	comment string
}

func (f *fakeGitHubAPI) AddLabels(_ context.Context, _, _ string, _ int, labels []string) error {
	f.labels = append(f.labels, labels...)
	return nil
}

func (f *fakeGitHubAPI) CreateIssueComment(_ context.Context, _, _ string, _ int, body string) error {
	f.comment = body
	return nil
}

func (f *fakeGitHubAPI) CreateCommitStatus(_ context.Context, _, _, _ string, status CommitStatus) error {
	f.status = status
	return nil
}

type fakeSNSPublisher struct {
	topicArn string
	message  string
}

func (f *fakeSNSPublisher) Publish(_ context.Context, input *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
	if input.TopicArn != nil {
		f.topicArn = *input.TopicArn
	}
	if input.Message != nil {
		f.message = *input.Message
	}
	return &sns.PublishOutput{}, nil
}

func pullRequestTestPayload(branch, title string) []byte {
	payload := map[string]any{
		"repository": map[string]any{
			"full_name": "sumanth-math/cvs",
		},
		"pull_request": map[string]any{
			"number": 42,
			"title":  title,
			"head": map[string]any{
				"ref": branch,
				"sha": "abc123",
			},
		},
	}
	data, _ := json.Marshal(payload)
	return data
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
