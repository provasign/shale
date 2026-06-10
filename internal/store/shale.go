// Package store owns the shale evidence format: the spec-v0 YAML structs
// (docs/03-shale-spec.md), the session JSONL event log under .shale/local/,
// and the append-only rules for finalized files.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the shale spec major version this build writes.
const SchemaVersion = "0"

// Privacy modes (docs/03-shale-spec.md §2).
const (
	PrivacyFull     = "full"
	PrivacyRedacted = "redacted"
	PrivacyHashOnly = "hash-only"
)

// File evidence provenance (spec rule 9).
const (
	ViaHook = "hook" // recorded by an agent hook adapter at edit time
	ViaGit  = "git"  // derived by finalize from git diff over the session window
)

// Shale is one agent session's evidence file (spec v0).
type Shale struct {
	ShaleVersion string      `yaml:"shale_version"`
	ID           string      `yaml:"id"`
	CreatedAt    time.Time   `yaml:"created_at"`
	FinalizedAt  time.Time   `yaml:"finalized_at"`
	Agent        Agent       `yaml:"agent"`
	Repo         Repo        `yaml:"repo"`
	Privacy      string      `yaml:"privacy"`
	Intent       *Intent     `yaml:"intent,omitempty"`
	Completion   *Completion `yaml:"completion,omitempty"`
	Transcript   *Transcript `yaml:"transcript,omitempty"`
	Files        []FileTouch `yaml:"files,omitempty"`
	Commands     []Command   `yaml:"commands,omitempty"`
	Notes        []Note      `yaml:"notes"`
	Redactions   int         `yaml:"redactions"`
}

type Agent struct {
	Tool           string `yaml:"tool"`
	ToolVersion    string `yaml:"tool_version,omitempty"`
	Model          string `yaml:"model,omitempty"`
	AdapterVersion string `yaml:"adapter_version,omitempty"`
}

type Repo struct {
	RootHint string `yaml:"root_hint"`
	Branch   string `yaml:"branch,omitempty"`
}

type Intent struct {
	Title       string    `yaml:"title"`
	Body        string    `yaml:"body,omitempty"`
	TitleSHA256 string    `yaml:"title_sha256"`
	BodySHA256  string    `yaml:"body_sha256,omitempty"`
	DeclaredAt  time.Time `yaml:"declared_at"`
	PromptCount int       `yaml:"prompt_count,omitempty"`
}

type Completion struct {
	Note           string  `yaml:"note,omitempty"`
	Model          string  `yaml:"model,omitempty"`
	TokensIn       int     `yaml:"tokens_in,omitempty"`
	TokensOut      int     `yaml:"tokens_out,omitempty"`
	TokensTotal    int     `yaml:"tokens_total,omitempty"`
	CostUSD        float64 `yaml:"cost_usd,omitempty"`
	PricingVersion string  `yaml:"pricing_version,omitempty"`
	Iterations     int     `yaml:"iterations,omitempty"`
}

type Transcript struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
	Kind   string `yaml:"kind"` // prompts (default) | full
}

type FileTouch struct {
	Path         string     `yaml:"path"`
	Ops          []string   `yaml:"ops"` // write | edit | delete
	Via          string     `yaml:"via"` // hook | git (spec rule 9)
	FirstTouched *time.Time `yaml:"first_touched,omitempty"`
}

type Command struct {
	Cmd        string    `yaml:"cmd"`
	ExitCode   *int      `yaml:"exit_code,omitempty"`
	At         time.Time `yaml:"at"`
	Classified string    `yaml:"classified"` // test | lint | scan | build | other
}

type Note struct {
	Text string    `yaml:"text"`
	At   time.Time `yaml:"at"`
}

// Validate enforces the spec invariants a writer must hold.
func (s *Shale) Validate() error {
	if s.ShaleVersion == "" {
		return errors.New("shale_version is required")
	}
	if s.ID == "" {
		return errors.New("id is required")
	}
	switch s.Privacy {
	case PrivacyFull, PrivacyRedacted, PrivacyHashOnly:
	default:
		return fmt.Errorf("invalid privacy mode %q", s.Privacy)
	}
	if s.Intent != nil && s.Intent.Title != "" && s.Intent.TitleSHA256 == "" {
		return errors.New("intent.title_sha256 is required when intent.title is set")
	}
	for _, f := range s.Files {
		if f.Via != ViaHook && f.Via != ViaGit {
			return fmt.Errorf("files[%s].via must be hook or git", f.Path)
		}
		if filepath.IsAbs(f.Path) {
			return fmt.Errorf("files[%s]: absolute paths are spec-invalid (rule 3)", f.Path)
		}
	}
	return nil
}

// Load reads and validates a shale YAML file.
func Load(path string) (*Shale, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse decodes shale YAML. Unknown fields are accepted (spec rule 1:
// forward-only); unknown major versions are rejected.
func Parse(raw []byte) (*Shale, error) {
	var s Shale
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse shale: %w", err)
	}
	if s.ShaleVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported shale_version %q (this build supports %q)", s.ShaleVersion, SchemaVersion)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the shale YAML. Finalized shale files are append-only (spec
// rule 2): Save refuses to overwrite an existing file.
func (s *Shale) Save(path string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — finalized shale files are append-only (spec rule 2)", path)
	}
	out, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
