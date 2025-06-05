package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/modfin/bellman/models"
	"github.com/modfin/bellman/prompt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/go-github/v56/github"
	"github.com/modfin/bellman"
	"github.com/modfin/bellman/models/gen"
	"github.com/modfin/bellman/schema"
	"github.com/urfave/cli/v3"
	"golang.org/x/oauth2"
)

// Config holds all configuration for the application
type Config struct {
	GithubToken            string
	Owner                  string
	Repo                   string
	PRNumber               int
	BellmanKey             string
	BellmanModel           gen.Model
	BellmanURL             string
	SystemPrompt           string
	BellmanMaxInputTokens  int
	BellmanMaxOutputTokens int
}

const DefaultSystemPrompt = `You are an expert code reviewer. 
Analyze the provided pull request and provide detailed, constructive feedback.
Focus on:
- Potential bugs and security issues
- Look for potential fat-fingers
- Don't be long winded, and focus on 1-3 key issues.
`

type PRReviewer struct {
	config *Config

	gh  *github.Client
	llm *bellman.Bellman
}

type Results struct {
	Summary     string   `json:"summary" json-description:"A summary of the entire pull request review, focus on summation of issues and suggestions. Don't explain what the change does, the author knows that'. Keep short, it to 1-3 sentences."`
	Issues      []Issue  `json:"issues"`
	Suggestions []string `json:"suggestions" json-description:"please provide suggestions for the pull request. Keep it to 0-3 bullet points with 1-3 sentences each."`
	Score       int      `json:"score" json-minimum:"1" json-maximum:"10" json-description:"please provide a score on the PR between 1 and 10. How good the PR is, where 0 is the worst and 10 is the best"`

	metadata models.Metadata `json:"-"`
}

type Issue struct {
	File        string `json:"file" json-description:"the file that the issue is in"`
	Line        int    `json:"line" json-description:"the line that the issue is on"`
	Type        string `json:"type" json-enum:"bug,style,performance,security"`
	Description string `json:"description" json-description:"a description of the issue"`
	Severity    string `json:"severity" json-enum:"low,medium,high"`
}

func main() {
	cmd := &cli.Command{
		Name:  "pre",
		Usage: "A PR review tool powered by LLMs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "github-token",
				Usage:    "GitHub API token",
				Sources:  cli.EnvVars("GITHUB_TOKEN"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "github-repository",
				Usage:    "GitHub repository in the format owner/repo",
				Sources:  cli.EnvVars("GITHUB_REPOSITORY"),
				Required: true,
			},
			&cli.IntFlag{
				Name:     "github-pr-number",
				Usage:    "Pull request number to review",
				Sources:  cli.EnvVars("GITHUB_PR_NUMBER"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "bellman-key",
				Usage:    "Bellman API key",
				Sources:  cli.EnvVars("BELLMAN_KEY"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "bellman-model",
				Usage:    "Bellman model to use (provider/model)",
				Value:    "VertexAI/gemini-2.0-flash",
				Sources:  cli.EnvVars("BELLMAN_MODEL"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     "bellman-url",
				Usage:    "Bellman API URL",
				Sources:  cli.EnvVars("BELLMAN_URL"),
				Required: true,
			},
			&cli.IntFlag{
				Name:    "bellman-max-input-tokens",
				Usage:   "The approximate maximum number of tokens to send to the LLM, if the pr is more, the bot refuse to eval the pr",
				Value:   10_000,
				Sources: cli.EnvVars("BELLMAN_MAX_INPUT_TOKENS"),
			},
			&cli.IntFlag{
				Name:    "bellman-max-output-tokens",
				Usage:   "The maximum number of tokens to receive from the LLM, if the pr is more, the bot refuse to eval the pr",
				Value:   5_000,
				Sources: cli.EnvVars("BELLMAN_MAX_OUTPUT_TOKENS"),
			},
			&cli.StringFlag{
				Name:    "system-prompt",
				Usage:   "Bellman system prompt to be used for PR review",
				Sources: cli.EnvVars("SYSTEM_PROMPT"),
				Value:   DefaultSystemPrompt,
			},
			&cli.StringFlag{
				Name:    "system-prompt-addition",
				Usage:   "Bellman system prompt addition to be used for PR review",
				Sources: cli.EnvVars("SYSTEM_PROMPT_ADDITION"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			slog.Default().Info("starting application")

			config := &Config{
				GithubToken:            cmd.String("github-token"),
				PRNumber:               cmd.Int("github-pr-number"),
				BellmanKey:             cmd.String("bellman-key"),
				BellmanURL:             cmd.String("bellman-url"),
				BellmanMaxInputTokens:  cmd.Int("bellman-max-input-tokens"),
				BellmanMaxOutputTokens: cmd.Int("bellman-max-output-tokens"),
				SystemPrompt:           cmd.String("system-prompt"),
			}
			var found bool
			config.Owner, config.Repo, found = strings.Cut(cmd.String("github-repository"), "/")

			if !found {
				return fmt.Errorf("invalid repository format: %s", cmd.String("repository"))
			}

			provider, model, found := strings.Cut(cmd.String("bellman-model"), "/")
			if !found {
				return fmt.Errorf("invalid model format: %s", cmd.String("bellman-model"))
			}
			config.BellmanModel = gen.Model{
				Provider: provider,
				Name:     model,
			}

			if len(strings.TrimSpace(config.SystemPrompt)) == 0 {
				config.SystemPrompt = DefaultSystemPrompt
			}
			config.SystemPrompt = strings.TrimSpace(config.SystemPrompt + "\n" + cmd.String("system-prompt-addition"))

			slog.Default().Info("loaded config")

			return runReview(config)
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Default().Error("got error running pre", "err", err)
		os.Exit(1)
	}
}

func runReview(config *Config) error {
	slog.Default().Info("starting PR review",
		"owner", config.Owner,
		"repo", config.Repo,
		"pr", config.PRNumber,
		"model", fmt.Sprintf("%s/%s", config.BellmanModel.Provider, config.BellmanModel.Name))

	if config.BellmanURL == "" {
		return fmt.Errorf("BELLMAN_URL is required")
	}

	// Initialize GitHub client
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.GithubToken})
	tc := oauth2.NewClient(context.Background(), ts)

	reviewer := &PRReviewer{
		gh:     github.NewClient(tc),
		config: config,
		llm: bellman.New(config.BellmanURL, bellman.Key{
			Name:  fmt.Sprintf("pre[%s/%s]", config.Owner, config.Repo),
			Token: config.BellmanKey,
		}),
	}

	return reviewer.ReviewPR(context.Background())
}

func (pr *PRReviewer) ReviewPR(ctx context.Context) error {
	slog.Default().Info("reviewing pull request", "pr", pr.config.PRNumber)

	// Get PR details
	pull, _, err := pr.gh.PullRequests.Get(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber)
	if err != nil {
		slog.Default().Error("failed to get PR details", "err", err)
		return fmt.Errorf("failed to get PR: %w", err)
	}
	slog.Default().Info("retrieved PR details", "title", pull.GetTitle(), "author", pull.GetUser().GetLogin())

	// Get PR diff
	diff, err := pr.getPRDiff(ctx)
	if err != nil {
		slog.Default().Error("failed to get PR diff", "err", err)
		return fmt.Errorf("failed to get PR diff: %w", err)
	}
	slog.Default().Info("retrieved PR diff", "diff_size", len(diff))

	// Get changed files
	files, err := pr.getChangedFiles(ctx)
	if err != nil {
		slog.Default().Error("failed to get changed files", "err", err)
		return fmt.Errorf("failed to get changed files: %w", err)
	}
	slog.Default().Info("retrieved changed files", "count", len(files))

	// Review with LLM
	slog.Default().Info("starting LLM review")
	review, err := pr.reviewWithLLM(ctx, pull, diff, files)
	if err != nil {
		slog.Default().Error("failed to review with LLM", "err", err)
		return fmt.Errorf("failed to review with LLM: %w", err)
	}
	slog.Default().Info("completed LLM review", "score", review.Score, "issues_count", len(review.Issues))

	// Post review comment
	slog.Default().Info("posting review summary comment")
	if err := pr.postReviewComment(ctx, review); err != nil {
		slog.Default().Error("failed to post review comment", "err", err)
		return fmt.Errorf("failed to post review comment: %w", err)
	}
	slog.Default().Info("posted review summary comment")

	// Post inline comments for specific issues
	if len(review.Issues) > 0 {
		slog.Default().Info("posting inline comments", "count", len(review.Issues))
		if err := pr.postInlineComments(ctx, review.Issues); err != nil {
			slog.Default().Error("failed to post inline comments", "err", err)
			return fmt.Errorf("failed to post inline comments: %w", err)
		}
		slog.Default().Info("posted all inline comments")
	} else {
		slog.Default().Info("no issues to post as inline comments")
	}

	slog.Default().Info("PR review completed successfully", "pr", pr.config.PRNumber)
	return nil
}

func (pr *PRReviewer) getPRDiff(ctx context.Context) (string, error) {
	slog.Default().Info("fetching PR diff")
	// Get the raw diff
	diff, _, err := pr.gh.PullRequests.GetRaw(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber, github.RawOptions{
		Type: github.Diff,
	})
	if err != nil {
		slog.Default().Error("failed to get PR diff", "err", err)
		return "", err
	}
	return diff, nil
}

func (pr *PRReviewer) getChangedFiles(ctx context.Context) ([]*github.CommitFile, error) {
	slog.Default().Info("fetching changed files")
	files, _, err := pr.gh.PullRequests.ListFiles(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber, nil)
	if err != nil {
		slog.Default().Error("failed to get changed files", "err", err)
		return nil, err
	}
	return files, nil
}

func (pr *PRReviewer) reviewWithLLM(ctx context.Context, pull *github.PullRequest, diff string, files []*github.CommitFile) (*Results, error) {
	slog.Default().Info("building review prompt")

	content := pr.buildReviewPrompt(pull, diff, files)
	slog.Default().Info("built review prompt", "length", len(content))

	if len(content) > pr.config.BellmanMaxInputTokens*4 {

		rev := "# LLM PR Review - too long\n\n" +
			"Review failed to to length of input, ~" + fmt.Sprintf("%d tokens", len(content)/4) + "\\\n" +
			"Maximum input length is ~" + fmt.Sprintf("%d tokens", pr.config.BellmanMaxInputTokens) + "\n\n"
		_, _, err := pr.gh.Issues.CreateComment(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber, &github.IssueComment{
			Body: &rev,
		})

		slog.Default().Error("review prompt too long", "length", len(content), "max", pr.config.BellmanMaxInputTokens*4)
		return nil, errors.Join(fmt.Errorf("review prompt many tokens: %d > %d", len(content)/4, pr.config.BellmanMaxInputTokens), err)
	}

	slog.Default().Info("system prompt", "prompt", pr.config.SystemPrompt)

	ress, err := pr.llm.Generator().
		Model(pr.config.BellmanModel).
		System(pr.config.SystemPrompt).
		MaxTokens(pr.config.BellmanMaxOutputTokens).
		Output(schema.From(Results{})).
		Prompt(prompt.AsUser(content))

	if err != nil {
		slog.Default().Error("failed to generate review",
			"err", err,
			"model", pr.config.BellmanModel,
			"url", pr.config.BellmanURL)
		return nil, fmt.Errorf("failed to generate review: %w", err)
	}

	slog.Default().Info("llm review completed",
		"model", ress.Metadata.Model,
		"input_tokens", ress.Metadata.InputTokens,
		"output_tokens", ress.Metadata.OutputTokens,
	)

	var result Results
	err = ress.Unmarshal(&result)
	if err != nil {
		slog.Default().Error("failed to unmarshal review result", "err", err)
		return nil, fmt.Errorf("failed to unmarshal review result: %w", err)
	}

	result.metadata = ress.Metadata
	return &result, nil
}

func (pr *PRReviewer) buildReviewPrompt(pull *github.PullRequest, diff string, files []*github.CommitFile) string {
	var sb strings.Builder

	sb.WriteString("Please review this pull request:\n\n")
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", pull.GetTitle()))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", pull.GetBody()))

	sb.WriteString("**Changed Files:**\n")
	for _, file := range files {
		sb.WriteString(fmt.Sprintf("- %s (+%d -%d)\n", file.GetFilename(), file.GetAdditions(), file.GetDeletions()))
	}

	sb.WriteString("\n**Diff:**\n```diff\n")
	sb.WriteString(diff)
	sb.WriteString("\n```")

	slog.Default().Info("built review prompt", "prompt", sb.String())

	return sb.String()
}

func (pr *PRReviewer) postReviewComment(ctx context.Context, review *Results) error {
	slog.Default().Info("formatting review comment")
	var comment strings.Builder

	// Add header with score
	comment.WriteString(fmt.Sprintf("# LLM PR Review - (Score: %d/10)\n\n", review.Score))

	comment.WriteString(fmt.Sprintf("Model: %s \\\n", review.metadata.Model))
	comment.WriteString(fmt.Sprintf("Input Tokens: %d \\\n", review.metadata.InputTokens))
	comment.WriteString(fmt.Sprintf("Output Tokens: %d\n\n", review.metadata.OutputTokens))

	// Add summary
	comment.WriteString("## Summary\n\n")
	comment.WriteString(review.Summary)
	comment.WriteString("\n\n")

	// Add issues section if there are any
	if len(review.Issues) > 0 {
		comment.WriteString("## Issues Found\n\n")
		for _, issue := range review.Issues {
			emoji := pr.getSeverityEmoji(issue.Severity)
			comment.WriteString(fmt.Sprintf("- %s **[%s]** `%s:%d` - %s\n",
				emoji, strings.ToUpper(issue.Type), issue.File, issue.Line, issue.Description))
		}
		comment.WriteString("\n")
	}

	// Add suggestions section if there are any
	if len(review.Suggestions) > 0 {
		comment.WriteString("## Suggestions\n\n")
		for i, suggestion := range review.Suggestions {
			comment.WriteString(fmt.Sprintf("%d. %s\n", i+1, suggestion))
		}
	}

	str := comment.String()

	_, _, err := pr.gh.Issues.CreateComment(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber, &github.IssueComment{
		Body: &str,
	})
	return err
}

func (pr *PRReviewer) postInlineComments(ctx context.Context, issues []Issue) error {
	slog.Default().Info("preparing inline comments", "issues_count", len(issues))
	// Get the PR to get the commit SHA
	pull, _, err := pr.gh.PullRequests.Get(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber)
	if err != nil {
		slog.Default().Error("failed to get PR", "err", err)
		return err
	}

	var comments []*github.DraftReviewComment

	for _, issue := range issues {
		if issue.File != "" && issue.Line > 0 {
			body := fmt.Sprintf("**%s Issue (%s)**\n\n%s",
				strings.Title(issue.Type), issue.Severity, issue.Description)

			issue.Line += 1 // Offset for line numbers

			comment := &github.DraftReviewComment{
				Path: &issue.File,
				Line: &issue.Line,
				Body: &body,
			}
			comments = append(comments, comment)
		}
	}

	if len(comments) > 0 {
		reviewRequest := &github.PullRequestReviewRequest{
			CommitID: pull.Head.SHA,
			Body:     github.String("Automated code review with inline comments"),
			Event:    github.String("COMMENT"),
			Comments: comments,
		}

		_, _, err := pr.gh.PullRequests.CreateReview(ctx, pr.config.Owner, pr.config.Repo, pr.config.PRNumber, reviewRequest)
		return err
	}

	return nil
}

func (pr *PRReviewer) getSeverityEmoji(severity string) string {
	switch strings.ToLower(severity) {
	case "high":
		return "🔴"
	case "medium":
		return "🟠"
	case "low":
		return "🟡"
	default:
		return "ℹ️"
	}
}
