package initx

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/provasign/shale/internal/store"
)

const shaleReadme = `# .shale/ — agent session evidence

These files are written by [Shale](https://github.com/provasign/shale): one
schema-versioned YAML per AI-agent session (intent, files touched, commands
run, token cost) plus redacted prompts-only transcripts. They are committed
so the evidence travels with the code and renders as a card on the PR.

- Finalized files are append-only; edits after capture are flagged on the card.
- ` + "`local/`" + ` is gitignored working state — never commit it.
`

// workflowYAML is the GitHub Actions workflow per architecture §3.6.
// SECURITY: pull_request_target + NO checkout, ever. Adding a checkout of PR
// code to this workflow reintroduces the classic privilege-escalation hole
// (ADR D12); shale doctor flags it.
const workflowYAML = `name: shale
on:
  pull_request_target:          # write-capable token for fork PRs — safe because
    types: [opened, synchronize, reopened]   # we never check out PR code
permissions:
  contents: read
  pull-requests: write
  checks: write
jobs:
  card:
    runs-on: ubuntu-latest
    steps:
      - uses: provasign/shale-action@v1   # no actions/checkout — everything via API
`

// Config is the committed .shale/config.yaml.
type Config struct {
	Privacy string `yaml:"privacy"`
}

// LoadConfig reads .shale/config.yaml, defaulting to redacted.
func LoadConfig(repoRoot string) Config {
	cfg := Config{Privacy: store.PrivacyRedacted}
	raw, err := os.ReadFile(filepath.Join(repoRoot, ".shale", "config.yaml"))
	if err == nil {
		_ = yaml.Unmarshal(raw, &cfg)
	}
	switch cfg.Privacy {
	case store.PrivacyFull, store.PrivacyRedacted, store.PrivacyHashOnly:
	default:
		cfg.Privacy = store.PrivacyRedacted
	}
	return cfg
}

// Scaffold creates the committed .shale/ skeleton and the workflow file.
// Idempotent; existing files are never overwritten.
func Scaffold(repoRoot, privacy string) ([]string, error) {
	var written []string
	files := map[string]string{
		filepath.Join(".shale", "README.md"):               shaleReadme,
		filepath.Join(".shale", "schema-version"):          store.SchemaVersion + "\n",
		filepath.Join(".shale", ".gitignore"):              "local/\n",
		filepath.Join(".shale", "config.yaml"):             "privacy: " + privacy + "\n",
		filepath.Join(".github", "workflows", "shale.yml"): workflowYAML,
	}
	for rel, content := range files {
		path := filepath.Join(repoRoot, rel)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", rel, err)
		}
		written = append(written, rel)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".shale", "local"), 0o755); err != nil {
		return written, err
	}
	return written, nil
}
