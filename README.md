# PREEN - PR Review Engine

[![PR Review with LLM](https://github.com/modfin/pre/actions/workflows/pr-review.yml/badge.svg)](https://github.com/modfin/pre/actions/workflows/pr-review.yml)

PREEN (PR Review Engine) is a GitHub Action that automatically reviews pull requests using Large Language Models (LLMs) through the Bellman library.

## Features

- Automated code review for pull requests
- Detailed feedback on potential bugs, security issues, and code quality
- Inline comments for specific issues
- Summary and scoring of pull requests
- Customizable system prompts

## Usage

Add this to your GitHub workflow file:

```yaml
name: PR Review with LLM

on:
  pull_request:
    types: [opened, synchronize, reopened]
  pull_request_review_comment:
    types:
      - created

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
        uses: modfin/pre
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          bellman-key: ${{ secrets.BELLMAN_KEY }}
          bellman-url: ${{ secrets.BELLMAN_URL }}
          bellman-model: 'VertexAI/gemini-2.0-flash'
```

## Configuration

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `github-token` | GitHub token for API access | Yes | `${{ github.token }}` |
| `bellman-key` | API key for LLM service | Yes | - |
| `bellman-url` | URL to Bellman service | Yes | - |
| `bellman-model` | LLM model to use for review | No | `VertexAI/gemini-2.0-flash` |

## Requirements

- A Bellman API key and URL
- GitHub token with permissions to read contents and write to pull requests

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Developed by

[Modular Finance](https://github.com/modfin)