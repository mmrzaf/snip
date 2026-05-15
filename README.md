# Snip

**snip** bundles source context into **deterministic** Markdown snapshots for AI tools, code review, or debugging.  
Deterministic means: same repo state + same config + same CLI arguments → same ordering, same inclusion decisions, same output.

---

## Install

### Go (recommended)

```bash
go install github.com/mmrzaf/snip/cmd/snip@latest
```

### Prebuilt binaries

Download the binary for your platform from the [Releases](https://github.com/mmrzaf/snip/releases) page and place it in your `$PATH`.

---

## Quick start

```bash
# Interactive setup
snip init

# Generate a snapshot using the default profile (from .snip.yaml)
snip

# Run a specific profile with runtime modifiers
snip run api
snip run api +tests -docs
snip run debug --stdout

# List files that would be included (dry‑run)
snip ls api

# Explain why a file is included or excluded
snip explain internal/app/snip.go

# Show effective configuration and diagnostics
snip doctor

# Apply AI‑generated markdown code blocks back to the filesystem
snip apply output.txt --file-header '===== FILE: {path} =====' --write
```

---

## Configuration (`.snip.yaml`)

Minimal example:

```yaml
version: 1
root: .
name: my-repo
default_profile: api

output:
  dir: .snip
  pattern: "snip_{profile}_{ts}_{gitsha}.md"
  latest: "last.md"
  stdout_default: false

render:
  format: md
  newline: "\n"
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
  max_chars: 120000
  per_file_max_lines: 600
  per_file_max_bytes: 262144
  drop_policy: drop_low_priority

ignore:
  use_gitignore: true
  always:
    - ".git/**"
    - "node_modules/**"
    - "dist/**"
    - "build/**"
    - ".venv/**"
    - ".snip/**"
  binary_extensions:
    - ".png"
    - ".jpg"
    - ".pdf"
    - ".zip"

sensitive:
  exclude_globs:
    - "**/.env*"
    - "**/*secret*"
    - "**/*key*"

slices:
  api:
    priority: 100
    include:
      - "internal/**"
      - "pkg/**"
      - "**/*.go"
    exclude:
      - "**/*_test.go"
  tests:
    priority: 40
    include:
      - "**/*_test.go"
      - "test/**"
  docs:
    priority: 20
    include:
      - "README.md"
      - "docs/**"

profiles:
  api:
    enable: ["api", "docs"]
  debug:
    enable: ["api", "tests", "docs"]
    budgets:
      max_chars: 200000
    render:
      tree_depth: 6
```

---

## Concepts

- **Slice** – a named group of files (e.g., `api`, `tests`, `docs`). Each slice defines `include`/`exclude` globs and a `priority`.
- **Profile** – a named snapshot recipe that enables a set of slices and may override budgets or rendering options.
- **Modifiers** – runtime toggles (`+slice` / `-slice`) applied on top of a profile. They do not change the config file.

A file can belong to multiple slices. It is included **once**, assigned to the slice with the highest priority, but all slice memberships are recorded in the manifest.

---

## Commands

### `snip init`

Creates a `.snip.yaml` configuration file.

- Scans the repository and generates sensible slices/profiles.
- Flags:
  - `--force` – overwrite existing config.
  - `--non-interactive` – use defaults without prompts.
  - `--profile-default <name>` – set a non‑persistent default profile hint.

### `snip run <profile> [modifiers...]`

Generates a snapshot bundle (default: write to a file).

Flags:
- `--config <path>` – path to config file (default `.snip.yaml`).
- `--root <path>` – override the root directory.
- `-o, --out <path>` – output file (use `-` for stdout).
- `--stdout` – shortcut for `-o -`.
- `--max-chars <n>` – override `budgets.max_chars`.
- `--format md` – only markdown supported (default).
- `--no-tree` – omit the repository tree.
- `--no-manifest` – omit the manifest sections.
- `--tree-depth <n>` – override `render.tree_depth`.
- `--include-hidden` – include hidden files (unless excluded by sensitive/ignore rules).
- `--quiet` – suppress printing the output path.

Exit codes:
- `0` – success.
- `2` – usage or configuration error.
- `3` – I/O error (e.g., cannot write output).
- `4` – partial output (some files excluded due to budgets, unreadable content, or invalid UTF‑8; snapshot still produced).

### `snip ls <profile> [modifiers...]`

Dry‑run: lists included files, their slice membership, and whether they would be truncated/dropped.

Flags:
- `--max-chars <n>` – override budget for the purpose of dry‑run.
- `--include-hidden` – include hidden files.
- `--verbose` – show detailed drop reasons.

### `snip doctor [modifiers...]`

Displays effective configuration, environment diagnostics, and top exclusion reasons.

Flags:
- `--profile <name>` – profile to use (defaults to config’s `default_profile`).
- `--include-hidden` – consider hidden files in exclusion statistics.

### `snip explain <path> [modifiers...]`

Explains why a specific file is included or excluded: discovery rules, slice matches, and effective selection.

Flags:
- `--profile <name>` – profile to evaluate.
- `--include-hidden` – treat hidden files as visible for matching.

### `snip apply <input-file>`

Parses a markdown (or text) file for code blocks delimited by a custom header and writes the extracted files.

Flags:
- `--file-header` – **required** header template containing `{path}` (e.g., `'===== FILE: {path} ====='`).
- `--write` – actually write files (default is dry‑run).
- `--force` – allow overwriting existing files.

The tool looks for the header line, then extracts the following code fence (any fence style) and its content. It handles nested fences correctly.

### `snip version`

Prints the version.

---

## Partial output (exit code 4)

When the snapshot is incomplete (e.g., due to budget cuts, unreadable files, or invalid UTF‑8), snip:
- still writes the bundle.
- prints warnings to stderr.
- exits with code `4`.

This is useful for automation where you want to accept an incomplete artifact but still be aware of omissions.

---

## Diagnostics examples

```bash
# See what would be included for profile 'api' with hidden files
snip ls api --include-hidden

# Understand why a file is excluded
snip explain .github/workflows/ci.yml --profile full

# Check effective budgets and top exclusion reasons
snip doctor --profile debug +tests
```

---

## Output pattern tokens

`output.pattern` supports:
- `{ts}` – timestamp `YYYYMMDD-HHMMSS`
- `{profile}` – profile name
- `{repo}` – base name of the root directory
- `{gitsha}` – short git SHA (or `000000` if not available)
- `{counter}` – monotonically increasing integer (stored in the output directory)

Repeated separators (`__`, `--`, `..`) are collapsed automatically.

---

## Contributing

See [`ARCHITECTURE.md`](./ARCHITECTURE.md) for design details, internal structure, and testing strategy.
