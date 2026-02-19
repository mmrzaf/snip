# Snip

## 1. Purpose

`snip` is a deterministic “source snapshot bundler” for feeding code context to AI tools. It produces a single, predictable bundle (Markdown in v1) containing:

- A header describing the snapshot
- An optional repository tree
- A manifest with file paths + line counts (and optional per-file metadata)
- The contents of selected files, grouped and sorted for scanability

Primary UX goal: **fast, predictable snapshots with minimal typing**:

- `snip run api +tests -docs` → build a bundle with profile `api`, adding `tests`, removing `docs`.
- Default output is **written to a file**. Stdout is opt-in via flag.

---

## 2. Non-Goals (v1)

- No LLM calls (init is heuristic + optional questions)
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

### 3.2 Predictability over Magic

Init may suggest slices/profiles, but runtime behavior is purely config-driven.

### 3.3 Debuggability

`snip` must allow users to answer:

- Why is file X included or excluded?
- What got trimmed due to budgets?
- What exactly was produced?

---

## 4. Key Concepts

### 4.1 Slice

A **Slice** is a named toggle representing a coherent chunk of repo context.
Examples: `api`, `tests`, `docs`, `schema`, `infra`, `cli`, `domain`, `persistence`, `configs`, `scripts`.

A slice defines:

- Include rules (paths/globs)
- Optional exclude rules
- Optional file type constraints
- Optional per-slice limits (max files, max bytes, truncation)

Users should not need to write regexes routinely; init should generate good path/glob mappings, and users should primarily adjust via:

- editing paths in config
- (optional v2) `snip add/remove <slice> <path>` helpers

### 4.2 Profile

A **Profile** is a named snapshot recipe:

- A baseline set of enabled slices
- Output settings (tree depth, manifest options)
- Budget settings (max chars, truncation)

Profiles are what users run most of the time: `api`, `tests`, `full`, `minimal`, `debug`.

### 4.3 Run Modifiers

Runtime toggles applied on top of a profile:

- `+slice` enables a slice for this run only
- `-slice` disables a slice for this run only

Modifiers do not persist and do not require separate local config.

---

## 5. CLI Contract (v1)

### 5.1 Commands

#### `snip init`

Creates `.snip.yaml` (or merges if requested).

- Scans repo structure and common signals
- Generates initial slices + profiles + ignore rules
- May ask optional questions if confidence is low or multiple archetypes detected
- Supports non-interactive mode (defaults)

Flags:

- `--root <path>` (default `.`)
- `--force` (overwrite existing config)
- `--non-interactive`
- `--profile-default <name>` (optional)

#### `snip run <profile> [modifiers...]`

Produces a snapshot bundle.

- Default: write to configured output file
- Stdout requires `--stdout` (or `-o -`)

Examples:

- `snip run api`
- `snip run api +tests -docs`
- `snip run full -infra`

Flags:

- `--config <path>` (default `.snip.yaml`)
- `--root <path>` (default from config or `.`)
- `-o, --out <path>` (override output path; `-` means stdout)
- `--stdout` (equivalent to `-o -`)
- `--format md` (v1 only)
- `--max-chars <n>` (override profile budget)
- `--no-tree`
- `--no-manifest`
- `--tree-depth <n>`
- `--include-hidden` (default false; hidden files excluded unless explicitly included)

Exit codes:

- `0` success
- `2` config/usage error
- `3` IO/permission error
- `4` partial run (some files unreadable; still produced output with warnings)

#### `snip ls <profile> [modifiers...]`

Dry-run list of included files and their slice membership; prints to stdout.

- Must show ordering and whether files would be truncated/dropped due to budgets.

Flags:

- same as `run` + `--verbose` (reasons)

#### `snip version`

Print version info.

(Recommended for v2: `snip explain <path>`, `snip add/remove`, `snip doctor`.)

---

## 6. Configuration Format (`.snip.yaml`)

### 6.1 Schema Overview

```yaml
version: 1

root: "." # optional, default project root
name: "" # optional friendly name

output:
  dir: ".snip" # default output directory
  pattern: "{ts}_{profile}_{gitsha}.md" # file name template
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

budgets:
  max_chars: 120000 # total output budget (rendered bundle chars)
  per_file_max_lines: 600
  per_file_max_bytes: 262144 # 256 KiB
  drop_policy: "drop_low_priority" # see §10

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
      max_chars: 120000
    render:
      tree_depth: 4

  full:
    enable: ["api", "tests", "docs"]
    budgets:
      max_chars: 220000
```

### 6.2 Output Pattern Tokens

`output.pattern` supports:

- `{ts}`: timestamp `YYYYMMDD-HHMMSS` (local time)
- `{profile}`: profile name
- `{repo}`: directory base name
- `{gitsha}`: short git SHA (empty if not a git repo)
- `{counter}`: optional monotonically increasing integer (see §9.3)

Rules:

- If a token is empty, it collapses to empty string; tool must also collapse repeated separators (e.g., `__` → `_`) optionally.

---

## 7. Init Flow (`snip init`)

### 7.1 Repo Scan Signals

Init searches for common signals to propose slices:

- OpenAPI/Swagger: `openapi.*`, `swagger.*`
- Protobuf: `**/*.proto`, `proto/**`
- GraphQL: `schema.graphql`, `**/*.graphql`
- DB migrations: `migrations/**`, `alembic/**`, `prisma/**`
- Common layouts:
  - Go: `cmd/**`, `internal/**`, `pkg/**`
  - Node: `src/**`, `routes/**`, `controllers/**`, `services/**`
  - Python: `app/**`, `src/**`, `tests/**`

### 7.2 Optional Questions (only if helpful)

Init may ask (all optional; defaults apply):

1. “Primary snapshot intent?” → `api` / `tests` / `full` / `infra` / `other`
2. “Prefer tree depth?” → 3/4/5
3. “Default budget?” → 120k/200k/custom
4. “Write latest file alias (last.md)?” → yes/no

Init never blocks on questions; Enter accepts defaults.

### 7.3 Init Output

Writes `.snip.yaml` with:

- global ignore + sensitive rules
- initial slices (minimum set: `api`, `tests`, `docs`, `schema`, `infra`, `configs`, `scripts`)
- profiles (`api`, `tests`, `full`, `minimal`, `debug`) using detected slices
- output config with defaults (file output enabled)

---

## 8. File Discovery & Ignore Rules

### 8.1 Root

All operations occur under `root`:

- CLI `--root` overrides config
- Root must be a directory

### 8.2 Ignore Evaluation Order

A candidate path is excluded if any applies:

1. outside root
2. matches `ignore.always` globs
3. matches `sensitive.exclude_globs`
4. (if enabled) matches `.gitignore` rules (including nested `.gitignore`s)
5. is binary (by extension OR by content sniffing)
6. is unreadable (permission, broken link) → excluded but recorded in manifest

### 8.3 Globbing

Use doublestar semantics (`**`) for cross-platform globbing.
All globs are evaluated on slash-normalized relative paths.

### 8.4 Binary Detection

Two-stage detection:

- Extension blacklist (fast)
- Content sniff: read first N bytes (e.g., 8 KiB). If contains NUL or high ratio of non-text → treat as binary.

Binary files:

- Excluded by default
- Listed in manifest as excluded (binary)

---

## 9. Selection Model

### 9.1 Slice Membership

A file belongs to a slice if it matches any `slice.include` and does not match `slice.exclude`.

A file can belong to multiple slices; bundle should:

- include the file **once**
- record all slice memberships in manifest

### 9.2 Profile Resolution

Effective enabled slices are:

1. profile `enable` list
2. apply run modifiers: `+slice`, `-slice` (run-only)
3. ignore unknown slices (error by default, or warning if `--lenient` future flag)

### 9.3 Output Path Resolution

Effective output path:

1. CLI `--out` (or `--stdout`)
2. else config `output.dir` + `output.pattern`

Default is file output.

Optional counter:

- If `{counter}` used, snip maintains `.snip/counter` as an integer, incremented atomically per run.

### 9.4 Atomic Writes

When writing to a file:

- write to temp: `<target>.tmp.<pid>`
- fsync (optional)
- rename to target
- update `output.latest` if configured (same atomic scheme)

---

## 10. Ordering, Grouping, and Determinism

### 10.1 Stable Ordering of Files in Bundle

Default ordering:

1. Group by slice (if `manifest.group_by_slice` and `render` groups content by slice)
2. Within each group: sort by relative path, ASCIIbetical, case-sensitive
3. If a file belongs to multiple enabled slices:
   - assign it to the highest-priority slice (highest `slice.priority`)
   - still list all slice memberships in manifest

Alternative mode (optional): global path sort without grouping.

### 10.2 Stable Ordering of Tree

Tree lists only directories/files that are in the final included set (recommended default).
Tree nodes sorted:

- directories first, then files
- lexicographic order

### 10.3 Budget and Truncation Determinism

When budget constraints apply, decisions must be deterministic:

- deterministic ordering of candidate files
- deterministic truncation rules (fixed max lines/bytes)
- deterministic drop policy

---

## 11. Budgets & Truncation (v1)

### 11.1 Budget Units

Primary: **rendered output char count** (`max_chars`).

### 11.2 Per-file Limits

Before assembling the final bundle:

- if file bytes > `per_file_max_bytes`: truncate
- if file lines > `per_file_max_lines`: truncate

Truncation strategy (default):

- keep head `N` lines, append truncation marker:
  `… [TRUNCATED: original_lines=1234 kept_lines=600]`

(Alternative strategies for v2: head+tail, middle cut.)

### 11.3 Global Budget Enforcement (`max_chars`)

If the assembled bundle exceeds `max_chars`, apply `drop_policy`.

#### `drop_low_priority` (default)

Drop entire slices from lowest `slice.priority` to highest until within budget.

- Within a slice, drop files last (v1 keeps it simple: drop whole slices).
- Record dropped slices/files in manifest with reason `budget_exceeded`.

If still too large after dropping all but highest slice:

- reduce per-file truncation further (e.g., halve `per_file_max_lines`) deterministically, and retry once.
- If still too large: hard cut bundle tail with marker and set exit code 4 (partial).

---

## 12. Rendering (Markdown v1)

### 12.1 Bundle Sections

1. **Header**
2. **Tree** (optional)
3. **Manifest** (optional)
4. **Content** (files)

### 12.2 Header Template (example)

```
# snip bundle

repo: my-service
root: .
profile: api
enabled_slices: [api, tests]
git_sha: a1b2c3d
timestamp: 2026-02-19T14:30:12+01:00
snip_version: 0.1.0
```

### 12.3 Manifest Format (AI-friendly)

Manifest must be scan-friendly and provide:

- ordinal index
- relative path
- line count
- byte count
- slice membership tags
- notes (truncated, unreadable, dropped)

Example:

```
## Manifest (included)

  1  internal/http/router.go             lines=210  bytes=8120   slices=[api]
  2  internal/http/handlers/user.go      lines=388  bytes=14502  slices=[api]
  3  tests/user_test.go                  lines=190  bytes=7021   slices=[tests]  truncated=false
```

Dropped:

```
## Manifest (dropped)

  - docs/architecture.md  reason=budget_exceeded slice=docs
  - scripts/seed.sh       reason=excluded_by_ignore pattern=scripts/**
```

### 12.4 File Block Format

Each included file is rendered as:

````
---

## 2) internal/http/handlers/user.go
lines: 388
bytes: 14502
slices: [api]
truncated: false

```go
<file content>
````

```

Rules:
- Always include the same metadata keys and ordering.
- Fence language inferred from extension (best-effort map), else no language.
- Normalize output newlines to `render.newline`.
- Preserve file content bytes as UTF-8 where possible; if not valid UTF-8, exclude and note.

---

## 13. Observability & Logging

### 13.1 User-Facing Output
- `run` should be silent on success except:
  - it prints the output path (unless `--quiet`)
- warnings printed to stderr:
  - unreadable files
  - invalid UTF-8 exclusions
  - budget drops

### 13.2 Debug Mode
`--verbose` prints:
- discovered file counts
- ignore reasons summary
- budgets and trimming decisions

---

## 14. Error Handling Rules

- Missing profile → usage error (exit 2)
- Unknown modifier slice (`+foo`) → usage error (exit 2) unless future `--lenient`
- Unreadable files → warn + manifest note + exit 4 (partial) if any unreadable in enabled slices
- Output path unwritable → exit 3 (no bundle produced)
- Config parse error → exit 2

---

## 15. Implementation Blueprint (Go)

### 15.1 Recommended Repo Layout
```

/cmd/snip/ # main
/internal/config/ # YAML schema + load/validate
/internal/initwizard/ # repo scan + optional prompts
/internal/discovery/ # file walking + ignore engine
/internal/selector/ # slice/profile resolution + modifiers
/internal/budget/ # truncation + budget enforcement
/internal/render/ # markdown rendering
/internal/gitinfo/ # git sha detection
/internal/util/ # path normalization, extensions, etc.

```

### 15.2 Key Dependencies (recommended)
- CLI: `cobra`
- YAML: `gopkg.in/yaml.v3`
- Glob: `github.com/bmatcuk/doublestar/v4`
- Git ignore parsing:
  - either implement minimal `.gitignore` support
  - or use `github.com/go-git/go-git/v5/plumbing/format/gitignore` (recommended)
- Git SHA: shell out to `git rev-parse --short HEAD` (fast, minimal) or use go-git

### 15.3 Core Data Structures (conceptual)
- `Config`
- `Slice { Name, IncludeGlobs, ExcludeGlobs, Priority, Limits }`
- `Profile { Name, EnabledSlices, BudgetOverrides, RenderOverrides }`
- `FileEntry { Path, AbsPath, Bytes, Lines, Slices[], Truncated, ExclusionReason }`
- `Plan { Included[], Dropped[], Stats, OutputPath }`

### 15.4 End-to-End Run Algorithm
1) Load + validate config
2) Resolve root and profile
3) Apply modifiers to enabled slices
4) Discover candidate files under root
5) Apply ignore/sensitive/gitignore/binary checks
6) Assign slice memberships
7) Build plan: included set (dedupe), compute metadata (bytes, lines)
8) Apply per-file truncation rules (deterministic)
9) Render candidate bundle and measure char count
10) If exceeds global budget:
    - apply drop policy deterministically
    - re-render
11) Write output (atomic) unless stdout
12) Print result path to stderr or stdout depending on UX choice (recommended: stderr)

---

## 16. Testing Strategy

### 16.1 Unit Tests
- glob matching behavior
- modifier parsing (`+/-`)
- ordering determinism
- truncation markers and metadata
- output pattern substitution

### 16.2 Golden Tests
Given a fixture repo:
- `snip run api` produces exact expected output (golden file)
- `snip run api +tests -docs` golden output

### 16.3 Cross-platform Considerations (Linux primary)
- Path normalization tests (ensure stable `/` separators in output)
- File permissions tests (unreadable paths)

---

## 17. Standard Slices & Profiles (Recommended Defaults)

### 17.1 Default Slices (universal)
- `api`
- `tests`
- `docs`
- `schema`
- `infra`
- `configs`
- `scripts`
- `cli`
- `domain`
- `persistence`

Init maps these to paths if found; otherwise slices exist but empty.

### 17.2 Default Profiles
- `api`: `api, schema, configs, docs`
- `tests`: `tests, api, domain`
- `full`: most slices except `infra` (optional)
- `minimal`: `api` only
- `debug`: `api, tests, schema, docs` with higher budgets

---

## 18. Future Enhancements (v2+)

- Scoring per file (tags) instead of slice-only drop policy
- `snip explain <path>` (why included/excluded)
- `snip add/remove <slice> <path>` (write config)
- TUI picker (`snip ui`)
- Zip output format
- More advanced truncation (head+tail)
- Optional secret scanning (warn-only) and safe redaction modes

---

## 19. Invariants (Rules That Must Always Hold)

1) Bundle ordering is deterministic.
2) Manifest is always correct and corresponds to content blocks.
3) Excluded sensitive files never appear in output.
4) Binary files never appear in output.
5) Output is atomic on disk (no partial files on crash).
6) `snip ls` matches what `snip run` would include, modulo budgets/truncation if budgets differ.

---
