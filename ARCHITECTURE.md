# Snip Architecture

## 1. Purpose

`snip` is a deterministic “source snapshot bundler” for feeding code context to AI tools. It produces a single, predictable bundle (Markdown in v1) containing:

- A header describing the snapshot
- An optional repository tree
- A manifest with file paths + line counts (and optional per-file metadata)
- The contents of selected files, grouped and sorted for scanability

Primary UX goal: **fast, predictable snapshots with minimal typing**:

- `snip run api +tests -docs` → build a bundle with profile `api`, adding `tests`, removing `docs`.
- Default output is **written to a file**. Stdout is opt-in via `--stdout` or `-o -`.

---

## 2. Non-Goals (v1)

- No LLM calls (init is heuristic; optional review is explicit)
- No “semantic understanding” of code
- No clipboard integration
- No background daemon or watchers
- No in-file redaction/masking (only exclusion rules)
- No rich UI/TUI (optional v2)

---

## 3. Design Principles

### 3.1 Determinism

Same repo state + same config + same CLI args → same included files and same ordering.

- Stable sorting rules
- Stable grouping
- Stable truncation rules
- Stable drop policy (drop_low_priority)

### 3.2 Predictability over Magic

Init may suggest slices/profiles, but runtime behavior is purely config-driven.

### 3.3 Debuggability

`snip` must allow users to answer:

- Why is file X included or excluded? (`snip explain`)
- What got trimmed due to budgets? (`snip doctor`, `snip ls --verbose`)
- What exactly was produced? (bundle content)

---

## 4. Key Concepts

### 4.1 Slice

A **Slice** is a named toggle representing a coherent chunk of repo context.
Examples: `api`, `tests`, `docs`, `schema`, `infra`, `cli`, `domain`, `persistence`, `configs`, `scripts`, `code`.

A slice defines:

- Include rules (paths/globs using doublestar)
- Optional exclude rules
- Priority (higher = more important)

Priority determines:
- Which slice “owns” a file when it belongs to multiple slices (highest priority wins).
- Order of dropping slices when budget is exceeded (lowest priority dropped first).

### 4.2 Profile

A **Profile** is a named snapshot recipe:

- A baseline set of enabled slices (`enable`)
- Optional budget overrides (`budgets.max_chars`)
- Optional render overrides (`render.tree_depth`)

Profiles are what users run most of the time: `api`, `full`, `minimal`, `debug`.

### 4.3 Run Modifiers

Runtime toggles applied on top of a profile:

- `+slice` enables a slice for this run only
- `-slice` disables a slice for this run only

Modifiers do not persist and do not require separate local config.

---

## 5. CLI Contract (v1)

### 5.1 Global flags

All commands support (where applicable):

- `--config <path>` – path to .snip.yaml (default: .snip.yaml or SNIP_CONFIG env)
- `--root <path>` – root directory override
- `--verbose` – enable debug logging

### 5.2 `snip init`

Creates `.snip.yaml` (or merges if requested).

- Scans repo structure and common signals
- Generates initial slices + profiles + ignore rules
- Non-interactive by default
- Runs interactive review only with `--interactive`

Flags:

- `--root <path>` (default `.`)
- `--force` (overwrite existing config)
- `--interactive`
- `--non-interactive` (compatibility alias; init is non-interactive by default)
- `--profile-default <name>` (optional)

### 5.3 `snip run <profile> [modifiers...]`

Produces a snapshot bundle.

- Default: write to configured output file
- Stdout requires `--stdout` (or `-o -`)

Examples:

- `snip run api`
- `snip run api +tests -docs`
- `snip run full -infra`

Flags:

- `--out, -o <path>` (override output path; `-` means stdout)
- `--stdout` (equivalent to `-o -`)
- `--max-chars <n>` (override profile budget)
- `--format md` (v1 only)
- `--no-tree`
- `--no-manifest`
- `--tree-depth <n>`
- `--include-hidden` (default false; hidden files excluded unless explicitly included)
- `--quiet` (do not print output path)

Exit codes:

- `0` success
- `2` config/usage error
- `3` IO/permission error
- `4` partial run (some files unreadable, invalid UTF-8, or budget drops; still produced output with warnings)

### 5.4 `snip ls <profile> [modifiers...]`

Dry-run list of included files and their slice membership; prints to stdout.

- Shows ordering and whether files would be truncated/dropped due to budgets.

Flags:

- same as `run` + `--verbose` (reasons for drops)

### 5.5 `snip doctor [modifiers...]`

Prints effective configuration and environment diagnostics.

Flags:

- `--profile <name>` (default from config)
- `--include-hidden`

Output includes:

- config path, root, profile, enabled slices
- budgets, git availability
- top exclusion reasons (counts)

### 5.6 `snip explain <path> [modifiers...]`

Explains why a path is included/excluded:

- discovery exclusion (ignore/sensitive/gitignore/binary/unreadable)
- slice include/exclude matches and which glob matched
- effective selection under the chosen profile/modifiers

### 5.7 `snip apply <input-file>`

Apply AI-generated markdown code blocks to the filesystem.

- Auto-detects Snip headers when `--file-header` is omitted.
- Supports a custom header template for nonstandard AI output.
- Extracts the following code fence content (handles nested fences correctly).
- Dry-run by default; `--write` actually writes files; `--force` allows overwriting.

Flags:

- `--file-header` (optional, e.g., `'===== FILE: {path} ====='`)
- `--write`
- `--force`

### 5.8 `snip version`

Print version (from `internal/app/version.go`, default `1.4.0`).

---

## 6. Configuration Format (`.snip.yaml`)

### 6.1 Schema Overview

```yaml
version: 1

root: "." # optional, default project root
name: "" # optional friendly name
default_profile: "api"

output:
  dir: ".snip" # default output directory
  pattern: "snip_{profile}_{ts}_{gitsha}.md" # file name template
  latest: "last.md" # optional: write/overwrite this file with latest snapshot
  stdout_default: false # default is file output

render:
  format: "md" # v1: md
  newline: "\n" # normalized output newline
  code_fences: true
  include_tree: true
  tree_depth: 4
  include_manifest: true
  manifest:
    group_by_slice: true
    include_line_counts: true
    include_byte_counts: true
    include_truncation_notes: true
    include_unreadable_notes: true
  file_block:
    header: "<<<FILE:{path}>>>"
    footer: ""

budgets:
  max_chars: 200000 # total output budget (rendered bundle chars)
  per_file_max_lines: 600
  per_file_max_bytes: 262144 # 256 KiB
  drop_policy: "drop_low_priority" # v1 only

ignore:
  use_gitignore: true
  always:
    - ".git/**"
    - "node_modules/**"
    - "dist/**"
    - "build/**"
    - ".venv/**"
    - ".pytest_cache/**"
    - "coverage/**"
    - "target/**"
    - ".snip/**"
  binary_extensions:
    - ".png"
    - ".jpg"
    - ".jpeg"
    - ".gif"
    - ".pdf"
    - ".zip"
    - ".tar"
    - ".gz"
    - ".7z"
    - ".exe"
    - ".dll"
    - ".so"
    - ".dylib"

sensitive:
  exclude_globs:
    - ".env*"
    - "**/*secret*"
    - "**/*secrets*"
    - "**/*.pem"
    - "**/*.key"
    - "**/id_rsa*"
    - "**/*serviceAccount*.json"

slices:
  api:
    include:
      - "internal/http/**"
      - "src/api/**"
    exclude: []
    priority: 100

  tests:
    include:
      - "tests/**"
      - "**/*test*/**"
    priority: 40

  docs:
    include:
      - "README*"
      - "docs/**"
    priority: 20

profiles:
  api:
    enable: ["api", "docs"]
    budgets:
      max_chars: 200000
    render:
      tree_depth: 4

  full:
    enable: ["api", "tests", "docs"]
    budgets:
      max_chars: 260000
```

### 6.2 Validation rules

- `version` must be 1
- `budgets.*` > 0
- `render.format` must be `"md"`
- `render.file_block.header` and `.footer` must not contain newlines
- At least one slice and one profile
- Profile `enable` entries must reference existing slices
- `default_profile` must exist
- `drop_policy` must be `"drop_low_priority"` (v1)

### 6.3 Output Pattern Tokens

`output.pattern` supports:

- `{ts}`: timestamp `YYYYMMDD-HHMMSS` (local time)
- `{profile}`: profile name
- `{repo}`: directory base name
- `{gitsha}`: short git SHA (or `000000` if not available)
- `{counter}`: optional monotonically increasing integer (stored in `.snip/counter`)

If a token is empty, it collapses to empty string; the tool collapses repeated separators (`__` → `_`, `--` → `-`, `..` → `.`).

---

## 7. Init Flow (`snip init`)

### 7.1 Project Detection (`detect.go`)

`detectProject` examines the root directory and returns:

- `Kind`: Go, Python, Node, Rust, Java, Ruby, PHP, .NET, or Unknown.
- `IsService`: presence of `cmd/`, `main.go`, `app.py`, `server.js`, etc.
- `IsLibrary`: absence of service indicators + typical library paths (`pkg/`, `src/`).
- `IsWebApp`: Node.js specific (vite, webpack, next, etc.)
- `HasTests`: presence of test directories or test file patterns.
- `HasDocs`: `README.md` or `docs/`.
- `HasConfigs`: any yaml/json/toml/ini files.
- `HasInfra`: `.github`, `Dockerfile`, `Makefile`, `justfile`, etc.

### 7.2 Slice Generation (`slices.go`)

Based on project kind and file scan:

- `code`: main source files, excluding tests (language‑specific patterns)
- `tests`: test files or directories
- `docs`, `configs`, `infra`, `scripts` – universal if matching files exist
- For web apps: `components`, `pages`, `styles`

If no slices match, a fallback `code` slice with `**/*` is created.

### 7.3 Profile Generation (`profiles.go`)

Builds four standard profiles:

- `api`: `code`, `docs`, `configs` (if exist)
- `full`: all slices
- `minimal`: only `code` (or first slice)
- `debug`: `api` + `tests`

### 7.4 Interactive Review (`prompts.go`)

Interactive review runs only with `--interactive`. The user sees the list of detected slices and their inclusion in the default profile. They can press Enter to accept or type modifiers (e.g., `+tests -configs`) to adjust the default profile’s enable list before writing.

### 7.5 Config Write

The generated config is validated and written with `util.AtomicWriteFile` plus a helpful header comment.

---

## 8. File Discovery & Ignore Rules (`internal/discovery`)

### 8.1 Root

All operations occur under `root`:

- CLI `--root` overrides config
- Root must be a directory (no symlinks allowed – rejected early)

### 8.2 Walk & Exclude Order

`Engine.Discover()` walks the tree using `filepath.WalkDir`. For each file/directory:

- Skip symlinks (if dir, `SkipDir`; if file, skip)
- Compute slash‑normalized relative path
- For directories: if they match `ignore.always` or gitignore (as directory pattern), `SkipDir`
- For files, apply exclusions in this order:

1. **`ignore.always`** – immediate exclusion
2. **`sensitive.exclude_globs`** – immediate exclusion
3. **Gitignore** (if `use_gitignore` enabled) – using `go-git/plumbing/format/gitignore`
4. **Binary detection**:
   - Extension match against `ignore.binary_extensions`
   - Content sniff: first 8KB, if contains NUL or >30% non‑printable ASCII → binary
5. **Unreadable** – stat or open fails → exclusion with reason `unreadable`

If none of the above, the file is **discovered** (not yet assigned to slices).

### 8.3 PathInfo Structure

```go
type PathInfo struct {
    RelPath         string
    AbsPath         string
    SizeBytes       int64
    IsHidden        bool          // any segment starts with '.'
    Excluded        bool
    ExclusionReason ExclusionReason
    ExclusionDetail string
}
```

Hidden files are marked but not automatically excluded; the selector handles them.

---

## 9. Selection Model (`internal/selector`)

### 9.1 Slice Membership

`membership()` determines which enabled slices a file belongs to:

- For each enabled slice, check if `rel` matches any `include` glob (doublestar).
- If match and no `exclude` glob matches.
- If file is hidden and `--include-hidden` is false, the file is only included if the matching include pattern explicitly starts with a dot segment (e.g., `".github/**"`).

`matchesAny` returns `(matched, explicitHidden)` – the second flag is true if the matching pattern starts with a dot segment.

### 9.2 Primary Slice

When a file belongs to multiple slices, the one with highest `priority` is the primary slice. Tie‑break by name.

### 9.3 Selected Output

`Select()` returns:

- `Included`: files that are discovered (not excluded) and have at least one enabled slice membership.
- `Dropped`: files that are discovered but excluded (by ignore/sensitive/gitignore/binary/unreadable) – they will appear in the manifest’s dropped section.

Sorting: both slices are sorted by `RelPath` lexicographically for deterministic baseline. Rendering may reorder based on `manifest.group_by_slice`.

---

## 10. Budgeting & Truncation (`internal/budget`)

### 10.1 Per‑file Limits

When building the plan (`BuildPlan`):

- Read file, count lines and bytes.
- If lines > `per_file_max_lines` or bytes > `per_file_max_bytes`:
  - Keep only the first `per_file_max_lines` lines.
  - Append a marker: `… [TRUNCATED: original_lines=X kept_lines=Y]`
- If file is not valid UTF‑8 → excluded with reason `invalid_utf8`.
- Any read error → excluded with reason `unreadable`.

Both cases mark `partial = true` (exit 4).

### 10.2 Global Budget Enforcement (`EnforceGlobalBudget`)

After rendering the initial plan:

1. If rendered length ≤ `max_chars` → done.
2. Otherwise, drop whole slices from lowest priority to highest (policy `drop_low_priority`):
   - Sort enabled slices by priority ascending (low to high).
   - Iterate, drop one slice at a time, re-render, check budget.
   - Never drop the last remaining slice.
3. If after dropping slices still over budget:
   - Halve `per_file_max_lines` (minimum 1) and rebuild the plan (re‑truncate all files).
   - Render again.
4. If still over budget:
   - Perform a **hard cut** of the rendered string at the remaining char limit and append `… [BUNDLE TRUNCATED: budget_exceeded]`.
   - Set `HardCut = true`.

All steps are deterministic.

### 10.3 Plan Structure

```go
type Plan struct {
    Profile       string
    EnabledSlices []string
    Included      []FileEntry
    Dropped       []DroppedEntry
    DroppedSlices []string
    Partial       bool
    HardCut       bool
}
```

`FileEntry` contains original/kept lines/bytes, content, truncation flag, and slice memberships.

---

## 11. Rendering (Markdown) (`internal/render`)

### 11.1 Renderer Configuration

```go
type Renderer struct {
    Newline         string
    CodeFences      bool
    IncludeTree     bool
    TreeDepth       int
    TreePaths       []string          // all discovered (non‑excluded) files for tree
    SlicePatterns   map[string]SlicePatterns
    IncludeManifest bool
    Manifest        ManifestOptions
    FileBlock       FileBlockOptions
}
```

### 11.2 Output Sections

1. **Header** (always):
   ```
   # snip bundle
   
   repo: ...
   root: ...
   profile: ...
   enabled_slices: [...]
   git_sha: ...
   timestamp: ...
   snip_version: ...
   ```

2. **Tree** (if `include_tree`):
   - Built from `TreePaths` (or from included files if not provided).
   - Shows directories first, then files, alphabetical.
   - Limited to `TreeDepth` (0 means unlimited? Actually depth default 4, if 0 treated as 1).
   - Uses ASCII tree drawing.

3. **Manifest (included)**:
   - Lists each included file with index, path, line/byte counts, slice membership, truncation flag.
   - Can be grouped by primary slice (`manifest.group_by_slice`).
   - Also prints `delimiter_header` / `delimiter_footer` if custom file block delimiters are used (for parser compatibility).

4. **Manifest (dropped)**:
   - Dropped slices (budget): `- slice=... reason=budget_exceeded include=... exclude=...`
   - Unused slices (enabled but no files matched): `reason=unused`
   - Not enabled slices: `reason=not_enabled`
   - Dropped files: `- path reason=... detail=... slice=...`

5. **File content**:
   - If custom delimiters are set (`file_block.header` or `.footer`), they are printed before/after the metadata and code fence.
   - Otherwise, uses standard `## N) path` format.
   - Metadata lines: `lines:`, `bytes:`, `slices:`, `truncated:`.
   - Code fence with language derived from extension (via `util.LanguageFromPath`).

### 11.3 Ordering

When `group_by_slice` is true:

- Files are grouped by primary slice.
- Slices are ordered by priority descending (higher first), then name.
- Within each slice, files sorted by path.

When false: global path sort.

This matches the specification.

---

## 12. Output Path Resolution & Atomic Writes

### 12.1 Resolution Order

1. CLI `--out` (if `-` → stdout)
2. else if `--stdout` → stdout
3. else use `output.dir` + `output.pattern`:
   - Tokens substituted via `util.ApplyPatternTokens`.
   - If `{counter}` present, `util.NextCounter` reads/writes `.snip/counter` in the output directory.
   - Output directory is created if missing.
   - Final filename cleaned (no path separators, forced `.md` extension if missing).

### 12.2 Atomic Write

- Write to `<target>.tmp.<pid>`
- `fsync` on parent directory (POSIX only, best effort)
- Rename to target
- If `output.latest` is set, write same content to that path (also atomic).

All writes use `util.AtomicWriteFile`.

---

## 13. Observability & Logging

- Normal `run` prints nothing to stdout except the output path (unless `--quiet`).
- Warnings printed to stderr:
  - “warning: snapshot is partial (some content excluded due to budget or errors)”
  - “warning: dropped slices (budget): ...”
  - “warning: X unreadable file(s) excluded”
  - “warning: X invalid UTF-8 file(s) excluded”
  - “warning: X file(s) dropped due to budget (use --verbose for details)”
  - “warning: bundle hard‑cut at max_chars limit”
- `--verbose` enables debug logs via `slog` (TextHandler) showing discovery counts, selection counts, etc.

`snip doctor` and `snip explain` produce human‑readable diagnostics.

---

## 14. Error Handling & Exit Codes

| Situation                                    | Exit code | Bundle produced? |
|----------------------------------------------|-----------|------------------|
| Missing config, parse error, validation error| 2         | no               |
| Unknown profile or slice modifier            | 2         | no               |
| Root not a directory / cannot stat           | 2         | no               |
| Output directory not writable                | 3         | no               |
| Some files unreadable / invalid UTF-8        | 4         | yes (partial)    |
| Budget forced slice drop or hard cut         | 4         | yes (partial)    |
| Success                                      | 0         | yes              |

`app.Wrap` attaches exit codes to errors; CLI `main` translates to `os.Exit(code)`.

---

## 15. Implementation Blueprint (Go) – current layout

```
cmd/snip/
├── apply.go
├── doctor.go
├── explain.go
├── init.go
├── ls.go
├── main.go
├── main_test.go
├── root.go
├── run.go
├── utils.go          # modifier escaping for dash modifiers
└── version.go

internal/
├── app/
│   ├── app_test.go
│   ├── doctor.go     # Doctor, Explain
│   ├── errors.go
│   ├── snip.go       # Run, List, write helpers
│   └── version.go
├── budget/
│   ├── budget.go     # Builder, Plan, truncation, global enforcement
│   └── budget_test.go
├── config/
│   ├── config.go     # Config struct, Load, Write, Validate, Default
│   ├── config_test.go
│   └── find.go       # FindConfigPath
├── discovery/
│   ├── discovery.go  # Engine, PathInfo, exclusion logic, binary sniff
│   └── discovery_test.go
├── gitinfo/
│   └── gitinfo.go    # ShortSHA
├── initwizard/
│   ├── detect.go     # Project detection
│   ├── initwizard.go # Run
│   ├── profiles.go   # buildProfiles
│   ├── prompts.go    # interactiveReview
│   ├── scan.go       # collectRepoFiles
│   ├── slices.go     # buildSlices
│   └── templates.go  # placeholder for future
├── render/
│   └── markdown.go   # Renderer, tree, manifest, file blocks
├── selector/
│   ├── selector.go   # Modifier parsing, membership, primary slice
│   └── selector_test.go
├── tools/
│   └── apply/
│       ├── apply.go      # Run, Options, Result
│       ├── apply_test.go
│       ├── output.go     # PlanSummary, VerbosePlan, WriteSummary
│       ├── parse.go      # Parse, header matching, fence handling
│       └── plan.go       # Apply, resolveTarget, effectiveRoot
└── util/
    ├── util.go           # NormalizeNewlines, LanguageFromPath, AtomicWriteFile, NextCounter, SniffBinary, FeedUTF8
    └── util_test.go
```

### Key Dependencies

- `github.com/spf13/cobra` – CLI framework
- `gopkg.in/yaml.v3` – YAML parsing
- `github.com/go-git/go-git/v5/plumbing/format/gitignore` – .gitignore parsing

---

## 16. End‑to‑End Run Algorithm

```
1. Load config (config.Load)
2. Apply root override (config.EffectiveRoot)
3. Resolve empty profile to `default_profile`
4. Apply profile overrides (config.ApplyProfileOverrides)
5. Parse modifiers (selector.ParseModifiers)
6. Compute enabled slices (selector.EnabledSlices)
7. Create discovery engine (discovery.NewEngine)
8. Discover files (eng.Discover)
9. Select included/dropped with slice membership (selector.Select)
10. Build plan with per‑file truncation (budget.Builder.BuildPlan)
11. If max_chars override, apply to limits
12. Render initial bundle (render.RenderMarkdown)
13. Enforce global budget (budget.Builder.EnforceGlobalBudget):
    - if under budget → done
    - drop low priority slices, re‑render
    - if still over, halve per_file_max_lines and rebuild
    - if still over, hard cut
14. Write output (atomic file or stdout)
15. Print warnings, return exit code via app.Wrap
```

---

## 17. Testing Strategy

### 17.1 Unit Tests (per package)

- `config`: loading, validation, defaults, atomic write.
- `discovery`: ignore order, binary detection, hidden detection.
- `selector`: modifier parsing, membership, hidden rule.
- `budget`: per‑file truncation, global drop policy, UTF-8 handling.
- `render`: manifest formatting, tree building, ordering.
- `apply`: parse fence blocks, duplicate detection, path traversal safety.
- `util`: token substitution, counter, atomic write.

### 17.2 Integration Tests (`app_test.go`)

- Full `Run` with temporary repositories: verify bundle content includes expected files, excludes others, and manifest contains correct slices.
- `Doctor` and `Explain` output contains expected strings.
- CLI argument escaping for dash modifiers (e.g., `-docs`).
- `Apply` command writing files with custom header.

### 17.3 Golden Tests (future)

- Fixture repositories with expected `.md` output.

### 17.4 Cross‑platform

- Path normalization (always slash in output).
- Atomic rename works on Windows (os.Rename is atomic on same filesystem).

---

## 18. Standard Slices & Profiles (as generated by init)

### Universal slices (detected by presence of matching files)

- `code` (priority 100) – main source code
- `tests` (priority 40) – test files
- `docs` (priority 20) – documentation
- `configs` (priority 15) – configuration files
- `infra` (priority 10) – CI/CD, Dockerfiles, Makefiles
- `scripts` (priority 5) – shell scripts

### Language‑specific code patterns

- Go: `**/*.go`, exclude `**/*_test.go`
- Python: `**/*.py`, exclude test patterns
- Node: `**/*.js`, `**/*.ts`, `**/*.jsx`, `**/*.tsx`, exclude test/spec
- etc.

### Profiles

- `api`: `code`, `docs`, `configs` (if exist)
- `full`: all slices
- `minimal`: only `code` (or first slice)
- `debug`: `api` + `tests`

---

## 19. Future Enhancements (v2+)

- More drop policies (score‑based, file‑level)
- `snip add/remove <slice> <path>` to edit config
- TUI picker (`snip ui`)
- Zip output format
- Advanced truncation (head+tail)
- Secret redaction (warning + optional masking)
- `--lenient` flag to ignore unknown modifiers

---

## 20. Invariants (Rules That Must Always Hold)

1. Bundle ordering is deterministic across runs.
2. Manifest always corresponds exactly to the content blocks.
3. Sensitive files (matched by `sensitive.exclude_globs`) never appear in output.
4. Binary files never appear in output.
5. Output is atomic on disk (no partial files on crash).
6. `snip ls` (with same budgets) matches the inclusion decisions of `snip run`.
7. Hidden files are excluded by default unless `--include-hidden` or explicit dot‑pattern.
8. Modifiers (`+/-`) never change the config file.
9. Every run that produces a bundle writes it atomically.
10. Partial runs (exit 4) still produce a usable snapshot (some content omitted with warnings).
