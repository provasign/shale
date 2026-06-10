# CI integrations

`shale render` is a plain binary that calls the GitHub API — it runs in any CI
that can download a binary, set environment variables, and make HTTPS calls.
The GitHub Action is convenience packaging, not architecture.

## Environment contract

Every CI integration (including the GitHub Action) sets the same three things:

| Variable | Where it comes from | Notes |
|---|---|---|
| `SHALE_TOKEN` | Your CI's secret store | A GitHub token; `GITHUB_TOKEN` is an accepted alias. Needs `repo:status` + PR read/write for the target repo |
| `SHALE_REPO` | Set in the pipeline or from CI env | `owner/repo` format; `GITHUB_REPOSITORY` is accepted as a fallback |
| PR number | `--pr <n>` flag | Required for all non–GitHub-Actions CIs; on GitHub Actions it is auto-detected from the event payload |

`shale render` fetches everything it needs — PR diff, `.shale/*.yaml` evidence
files — from the GitHub API. It **never reads the local filesystem** for
evidence and never needs a checkout of any branch.

---

## GitHub Actions (default)

`shale init` writes `.github/workflows/shale.yml` automatically. Nothing
extra to do.

```yaml
name: shale
on:
  pull_request_target:
    types: [opened, synchronize, reopened]
permissions:
  contents: read
  pull-requests: write
  checks: write
jobs:
  card:
    runs-on: ubuntu-latest
    steps:
      - uses: provasign/shale-action@v1
```

The action downloads the Shale binary (checksum-verified), then runs
`shale render`. The `GITHUB_TOKEN` available in the job context is sufficient —
no PAT is needed for public or private repos in the same org.

**Why `pull_request_target`?** This event runs the workflow from the **default
branch** (not the PR branch), giving it a write-capable token that works for
fork PRs. Because Shale never checks out PR code, this is safe. Adding an
`actions/checkout` step to this workflow would reintroduce a classic
privilege-escalation hole — `shale doctor` flags it if it detects one.

### Pinning a specific version

The `v1` tag always points to the latest release. To pin:

```yaml
      - uses: provasign/shale-action@v1
        with:
          version: "v0.1.13"
```

### GitHub Enterprise Server

Set `SHALE_REPO` to `owner/repo` explicitly if `GITHUB_REPOSITORY` is not set
by your runner, and point the API URL at your GHES instance:

```yaml
    env:
      SHALE_REPO: ${{ github.repository }}
      SHALE_API_URL: https://ghes.example.com/api/v3
```

> `SHALE_API_URL` support is confirmed in the driver; `GITHUB_REPOSITORY` is
> usually set by standard GHES runner images, but SHALE_REPO takes precedence
> if both are set.

---

## Jenkins

Install the Shale binary on your Jenkins agents (or download it per-build as
shown below), store a GitHub PAT as a Jenkins credential, then add a
conditional stage to your `Jenkinsfile`.

### Required GitHub token scope

Create a fine-grained PAT (or classic PAT with `repo` scope) that can:
- Read pull requests and their file lists
- Post PR comments (`pull-requests: write` equivalent)
- Post commit statuses or check runs

Store it as a Jenkins secret text credential with id `shale-github-token`.

### Multibranch pipeline (recommended)

Jenkins multibranch pipelines set `$CHANGE_ID` to the PR number on PR builds.

```groovy
pipeline {
  agent any

  stages {
    stage('Build and test') {
      steps {
        sh 'make test'
      }
    }

    stage('Shale card') {
      // Only runs on pull request builds
      when { changeRequest() }
      steps {
        // Download shale binary (or install once on the agent and skip this)
        sh '''
          SHALE_VERSION=v0.1.13
          ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
          curl -fsSL "https://github.com/provasign/shale/releases/download/${SHALE_VERSION}/shale_linux_${ARCH}.tar.gz" \
            | tar -xz shale
          chmod +x shale && mv shale /tmp/shale-bin
        '''

        withCredentials([string(credentialsId: 'shale-github-token', variable: 'SHALE_TOKEN')]) {
          sh "/tmp/shale-bin render --pr ${env.CHANGE_ID}"
        }
      }
    }
  }
}
```

`SHALE_REPO` defaults to `GITHUB_REPOSITORY` if set by your Jenkins GitHub
plugin. If not, set it explicitly:

```groovy
        withCredentials([string(credentialsId: 'shale-github-token', variable: 'SHALE_TOKEN')]) {
          withEnv(["SHALE_REPO=your-org/your-repo"]) {
            sh "/tmp/shale-bin render --pr ${env.CHANGE_ID}"
          }
        }
```

### Installing on the agent instead of downloading per-build

Download and install the binary once when provisioning the Jenkins agent:

```sh
SHALE_VERSION=v0.1.13
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL "https://github.com/provasign/shale/releases/download/${SHALE_VERSION}/shale_linux_${ARCH}.tar.gz" \
  | tar -xz -C /usr/local/bin shale
```

Then drop the download step from the `Jenkinsfile`.

### Fail-open note

`shale render` exits `0` even when it posts nothing (no evidence on the PR).
The stage will never fail a Jenkins build. Do not use it as a gate.

---

## CircleCI

```yaml
jobs:
  shale-card:
    docker:
      - image: cimg/base:stable
    steps:
      - run:
          name: Download shale
          command: |
            SHALE_VERSION=v0.1.13
            ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
            curl -fsSL "https://github.com/provasign/shale/releases/download/${SHALE_VERSION}/shale_linux_${ARCH}.tar.gz" \
              | tar -xz -C /usr/local/bin shale
      - run:
          name: Render Shale card
          command: shale render --pr "$CIRCLE_PR_NUMBER"
          environment:
            SHALE_REPO: your-org/your-repo
            # SHALE_TOKEN: set in CircleCI project environment variables

workflows:
  pr-evidence:
    jobs:
      - shale-card:
          filters:
            branches:
              ignore: main   # only on PR branches
```

Set `SHALE_TOKEN` as a CircleCI environment variable in Project Settings →
Environment Variables.

---

## GitLab CI

GitLab CI support is on the MVP 2 roadmap (`SHALE_FORGE=gitlab`). For now,
if your code is hosted on GitHub but CI runs on GitLab, use the GitHub token
approach above — the renderer only needs GitHub API access; it doesn't care
where CI runs.

---

## Generic pipeline / custom script

Any pipeline that can run a shell command and set environment variables:

```sh
#!/usr/bin/env bash
set -euo pipefail

# Download (or assume pre-installed on agent)
SHALE_VERSION=v0.1.13
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL "https://github.com/provasign/shale/releases/download/${SHALE_VERSION}/shale_linux_${ARCH}.tar.gz" \
  | tar -xz -C /usr/local/bin shale

# Required: token + repo + PR number
export SHALE_TOKEN="${GITHUB_TOKEN:?need GITHUB_TOKEN}"
export SHALE_REPO="${GITHUB_REPOSITORY:?need GITHUB_REPOSITORY}"   # owner/repo
PR_NUMBER="${1:?pass PR number as first arg}"

shale render --pr "$PR_NUMBER"
```

The binary exits `0` on success (card posted or nudge posted) and `1` on
configuration or API errors. It never exits nonzero because a PR has no
evidence.

---

## Troubleshooting CI runs

**`SHALE_REPO (owner/repo) is required`**  
Neither `SHALE_REPO` nor `GITHUB_REPOSITORY` is set. Set `SHALE_REPO=owner/repo`
in your pipeline environment.

**`SHALE_TOKEN or GITHUB_TOKEN is required`**  
The token isn't reaching the shell. Check that your secret is mapped to the
environment variable, not just stored.

**`shale render: --pr <n> required`**  
The pipeline isn't on GitHub Actions (where PR number is auto-detected). Pass
`--pr $YOUR_CI_PR_VAR` explicitly.

**`github POST /repos/…/check-runs: 403`**  
The token doesn't have `checks: write`. Shale falls back to a commit status
automatically (the card comment still posts). For full check-run support,
grant `repo:status` on the PAT or add `checks: write` to the token's fine-grained permissions.

**Card posts but shows "No shale for this PR"**  
The `.shale/` directory exists on the default branch but there are no session
YAML files on the PR branch's HEAD. This usually means `shale finalize` didn't
run before push — check that the pre-push hook is installed (`shale doctor`)
and that the evidence commit was included in the push.
