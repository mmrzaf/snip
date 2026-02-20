# snip

**snip** bundles source context into **deterministic** Markdown snapshots for AI/code review/debugging.

Deterministic means: given the same repo state + same config + same CLI args, snip produces the **same ordering and decisions**:

- stable discovery ordering (sorted relpaths)
- stable slice membership resolution
- stable truncation + budget enforcement
- stable rendering order

---

## Install

### Go (recommended)

```bash
go install github.com/mmrzaf/snip/cmd/snip@latest
```

### Prebuilt binaries

Download the appropriate binary for your platform from the **Releases** page and place it in your `$PATH`.

---

## Quick start

Initialize config:

```bash
snip init
```

Run a default snapshot (uses `default_profile` from `.snip.yaml`):

```bash
snip
```

Run an explicit profile:

```bash
snip run api
snip run debug
```

Toggle slices at runtime:

```bash
snip run api +tests
snip run debug -docs +configs
```

---

## Config: minimal example

`.snip.yaml`

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
  binary_extensions: ["png", "jpg", "pdf", "zip"]

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
    exclude: []

  docs:
    priority: 20
    include:
      - "README.md"
      - "docs/**"
    exclude: []

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

## Slices and profiles

- A **slice** is a named file set (`include` globs minus `exclude` globs) with a priority.
- A **profile** enables a list of slices and can override certain budgets/render settings.

A file can match multiple slices. snip includes it **once**, but records all memberships in the manifest.

Runtime modifiers:

- `+slice` enables a slice for this run
- `-slice` disables a slice for this run

Examples:

```bash
# add tests just for this run
snip run api +tests

# strip docs for debugging-focused snapshot
snip run debug -docs
```

---

## Common workflows

### Bundle for PR review

Goal: include core code + docs, skip heavy test/infra noise.

```bash
snip run api
```

### Bundle for debugging

Goal: include tests/configs and deeper tree visibility.

```bash
snip debug +configs +tests
```

---

## Partial output behavior (exit code 4)

snip returns:

- `0` success
- `2` usage/config error
- `3` IO error
- `4` **partial output** (snapshot was produced, but exclusions/truncation occurred)

Partial output happens when:

- unreadable files were excluded
- invalid UTF-8 files were excluded
- global budget forced dropping slices/files
- bundle was hard-cut due to `max_chars`

When partial output occurs:

- snip still writes the snapshot (file or stdout)
- warnings are printed to stderr (`warning: ...`)
- process exits with code **4**

This is intended for CI/automation: you can treat `4` as "artifact produced but incomplete".

---

## Diagnostics

### snip doctor

Prints:

- effective config path
- effective root
- enabled slices
- effective budgets
- git availability
- top exclusion reasons

```bash
snip doctor
snip doctor --profile debug +tests
```

### snip explain <path>

Explains:

- discovery exclusion (ignore/sensitive/gitignore/binary/unreadable)
- slice include/exclude matches and which glob matched
- effective selection under the chosen profile/modifiers

```bash
snip explain internal/app/snip.go
snip explain .github/workflows/ci.yml
snip explain internal/app/snip.go +tests
```
