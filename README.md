# PRE - PR Engine

[![PR Review with LLM](https://github.com/modfin/pre/actions/workflows/pr-review.yml/badge.svg)](https://github.com/modfin/pre/actions/workflows/pr-review.yml)
[![PR Bot with LLM](https://github.com/modfin/pre/actions/workflows/pr-bot.yml/badge.svg)](https://github.com/modfin/pre/actions/workflows/pr-bot.yml)

PRE (PR Engine) is a GitHub Action that automatically reviews pull requests using Large Language Models (LLMs) through the Bellman library.

## Features

- Automated code review for pull requests when they are opened
- On-demand reviews triggered by commenting `/bellman` on a pull request
- Detailed feedback on potential bugs, security issues, and code quality
- Inline comments for specific issues
- Summary and scoring of pull requests
- Customizable system prompts

## Usage

See examples in `.github/workflows` directory

### Automatic PR Review

Add this to your GitHub workflow file to automatically review PRs when they are opened:

```yaml
name: Bellman PR Evaluator

on:
  pull_request:
    types: [opened]

jobs:
  pre-llm-review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Review PR with LLM
        uses: modfin/pre@master
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          bellman-key: ${{ secrets.BELLMAN_KEY }}
          bellman-url: ${{ secrets.BELLMAN_URL }}
          bellman-model: 'VertexAI/gemini-2.0-flash'
```

### On-Demand PR Review

Add this to your GitHub workflow file to enable on-demand reviews triggered by commenting `/bellman` on a pull request:

```yaml
name: Bellman PR Bot
on:
  issue_comment:
    types:
      - created

jobs:
  pr-bot-on-demand:
    # Only run on pull request comments that start with '/bellman'
    if: github.event.issue.pull_request && startsWith(github.event.comment.body, '/bellman')
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          # Checkout the PR branch, not the base branch
          ref: ${{ github.event.pull_request.head.sha }}
          fetch-depth: 0

      - name: Review PR with LLM
        uses: modfin/pre@master
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          bellman-key: ${{ secrets.BELLMAN_KEY }}
          bellman-url: ${{ secrets.BELLMAN_URL }}
          bellman-model: 'VertexAI/gemini-2.0-flash'
          system-prompt-addition: ${{ github.event.comment.body }}
```

## Configuration

| Input | Description                         | Required | Default                     |
|-------|-------------------------------------|----------|-----------------------------|
| `github-token` | GitHub token for API access         | Yes | `${{ github.token }}`       |
| `bellman-key` | API key for LLM service             | Yes | -                           |
| `bellman-url` | URL to Bellman service              | Yes | -                           |
| `bellman-model` | LLM model to use for review         | No | `VertexAI/gemini-2.0-flash` |
| `system-prompt` | Sets a specific system prompt      | No | see `DefaultSystemPrompt`  |
| `system-prompt-addition` | Additional instructions for the LLM | No | -                           |

## Requirements

- A Bellman API key and URL
- GitHub token with permissions to read contents and write to pull requests

## Further Development Suggestions

- Add Agent mode allowing the LLM to eg read the entire files, and not just the diffs, if they want to.
- Refactor, cleanup and make testable
- Add built binaries instead of building from source each action\
eg https://full-stack.blend.com/how-we-write-github-actions-in-go.html

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Developed by

[Modular Finance](https://github.com/modfin)