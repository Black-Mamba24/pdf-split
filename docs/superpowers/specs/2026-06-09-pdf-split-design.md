# pdf-split Design

## Summary

`pdf-split` is a local, cross-platform Go CLI that splits one PDF into ordered,
continuous page ranges. It supports a requested minimum number of output files,
a strict maximum output-file size, or both.

The tool guarantees that every input page appears exactly once in the outputs,
in the original order. It preserves page content, page dimensions, and page
rotation. It does not guarantee preservation of document-level features such as
bookmarks, forms, attachments, metadata, or digital signatures.

Version 1 targets PDFs up to approximately 10,000 pages and 10 GB. It does not
support encrypted PDFs.

## CLI Contract

```text
pdf-split <input.pdf> [--parts N] [--max-size SIZE] [--output DIR] [--overwrite] [--no-progress]
pdf-split --help
pdf-split -h
```

Examples:

```bash
pdf-split report.pdf --parts 4
pdf-split report.pdf --max-size 10MB --output ./result
pdf-split report.pdf --parts 4 --max-size 10MB --overwrite
```

Arguments and options:

- `<input.pdf>` accepts an absolute or relative path to one PDF.
- `--parts N` sets the requested minimum output count.
  - Used alone, it produces exactly `N` files with page counts as equal as
    possible. Earlier files receive the remainder pages.
  - `N` must be between 1 and the input page count.
- `--max-size SIZE` sets a strict actual on-disk size limit.
  - It accepts positive integers or decimals with case-insensitive `KB`, `MB`,
    or `GB`.
  - Units are binary: `1 KB = 1024 bytes`, `1 MB = 1024 KB`, and
    `1 GB = 1024 MB`.
  - A single page larger than the limit is emitted as an oversized one-page
    output, produces a warning on standard error, and does not make the command
    fail.
- `--output DIR` defaults to the current working directory. Missing directories
  and parents are created.
- `--overwrite` permits replacing conflicting output files. Existing files stay
  unchanged unless all new files have been generated and verified.
- `--no-progress` disables dynamic progress output.
- `--help` and `-h` print usage, defaults, size-unit rules, combined-constraint
  behavior, oversized-page behavior, and examples, then exit successfully.
- At least one of `--parts` and `--max-size` is required.

When both constraints are present, the output count must not be less than
`--parts`. The tool uses the smallest output count that also satisfies
`--max-size`, then balances output sizes within that count.

Outputs are named from the input basename:

```text
<input-basename>-001.pdf
<input-basename>-002.pdf
```

Numbering is zero-padded to at least three digits and expands when required.
Existing conflicting files cause an error unless `--overwrite` is present.

## Core Invariants

Every plan consists only of inclusive, one-based, continuous page ranges:

```go
type PageRange struct {
    Start int
    End   int
}

type SplitPlan struct {
    Ranges []PageRange
}
```

Every created or modified plan must pass these checks:

- The first range starts at page 1.
- Every range contains at least one page.
- Each range starts exactly one page after the previous range ends.
- The last range ends at the input page count.

These invariants guarantee no missing pages, duplicate pages, reordered pages,
or empty outputs. Final verification also confirms that each output's page count
matches its planned range and that the sum of output page counts equals the
input page count.

The guarantee is page-level, not byte-level. Splitting rewrites PDF structure,
but page order and visual content must remain unchanged.

## Architecture

The implementation is divided into focused Go packages:

- `cmd/pdf-split`: executable entry point, exit status, and signal handling.
- `internal/cli`: argument parsing, help output, size parsing, and validation.
- `internal/pdf`: PDF validation, encryption detection, page count, and writing
  a continuous page range.
- `internal/planner`: creates and adjusts valid `SplitPlan` values.
- `internal/measure`: writes temporary candidate PDFs, measures actual sizes,
  and maintains a bounded measurement cache.
- `internal/verify`: validates plan coverage, output page counts, readability,
  actual sizes, and ordering.
- `internal/output`: naming, temporary workspace, conflict checks, safe
  replacement, rollback, and cleanup.
- `internal/progress`: terminal detection and progress rendering to standard
  error.

The PDF engine is hidden behind an internal interface so it can be replaced
without changing planning or output behavior. The initial engine will use
`pdfcpu`, an Apache-2.0 Go library that exposes validation, split, and trim
APIs. Because pdfcpu identifies itself as alpha software, implementation must
include compatibility fixtures and a scale benchmark before release.

Only dependencies under permissive licenses such as MIT, Apache-2.0, or BSD are
allowed.

## Planning Algorithms

### Parts Only

For total pages `P` and requested parts `N`:

- Reject `N > P`.
- Assign `floor(P/N)` pages to every output.
- Assign one additional page to each of the first `P mod N` outputs.

This produces exactly `N` continuous ranges whose page counts differ by at most
one.

### Maximum Size

PDF output size is not additive by page because shared fonts, images, and other
resources may be duplicated or removed when a range is written. A strict size
limit therefore requires generating and measuring candidate PDFs.

The planner uses adaptive boundary search:

1. Starting at the next unassigned page, expand the candidate end page to find
   an interval containing the largest range that fits.
2. Use binary search within that interval.
3. Check measurements around the selected boundary. PDF range sizes are not
   guaranteed to be monotonic, so fall back to a bounded linear scan whenever
   the measurements contradict the binary-search assumption.
4. Cache measured `(start, end)` ranges in a bounded cache.
5. Repeat until every page is assigned. This establishes a valid plan with the
   smallest output count found by the bounded adaptive search.
6. Move boundaries between adjacent ranges to reduce size imbalance while
   preserving all invariants and the size limit.
7. Stop local balancing when no useful move exists or the configured
   measurement and iteration budgets are exhausted.

The result is efficient and approximately balanced; mathematical global
optimality, including proof of the globally minimum output count, is not
required. Candidate files are placed in a temporary directory and deleted
promptly. The complete input and all candidates are never retained in memory
together.

The final outputs are regenerated and their actual on-disk sizes are strictly
verified. If any non-single-page output exceeds the limit, its boundary is
reduced and the affected suffix is replanned.

### Combined Constraints

The planner first determines the smallest satisfying count found by the bounded
adaptive search for `--max-size`. The final count is the larger of that value
and `--parts`. It then creates that many continuous ranges and balances their
actual sizes without allowing any non-single-page output to exceed the limit.

## Progress Reporting

Dynamic progress is enabled by default only when standard error is an
interactive terminal. It is disabled when output is redirected or
`--no-progress` is supplied.

Planning displays non-percentage activity because the final measurement count is
not known in advance:

```text
Planning split boundaries... 42 measurements completed
```

Final generation displays one retained line per output and calculates the
current percentage from bytes written compared with the size measured during
planning. For `--parts`-only runs, which do not otherwise require measurement,
the tool uses an estimated target and holds the display below 100% until the
file is closed and verified. Progress is informational and never participates
in correctness decisions:

```text
[1/4] report-001.pdf  #################### 100%  8.4MB
[2/4] report-002.pdf  ############--------  62%  5.1MB
```

Progress and warnings go to standard error so standard output remains usable by
scripts. On success, standard output receives a concise summary containing the
output directory, file count, total page count, and warning count.

## Safe Output and Failure Handling

The tool creates a temporary workspace on the same filesystem as the output
directory. It generates and verifies every result there before publishing.

Without `--overwrite`, any target-name conflict fails after planning determines
the output count and before final output generation. With `--overwrite`,
existing target files are backed up immediately before publication. New files
are renamed into place only after all outputs pass verification. If publication
fails, already replaced files are removed and backups are restored.

Multiple output files cannot be replaced by one filesystem-atomic operation.
The design therefore provides rollback semantics and minimizes the interval in
which a partial new set is visible.

Errors and interrupt signals remove this run's temporary files. They do not
leave partial outputs and do not destroy pre-existing outputs.

Exit statuses:

```text
0  Success, including oversized single-page warnings
2  Invalid CLI arguments
3  Missing, unreadable, or invalid input PDF
4  Unsupported PDF, including encryption
5  Unsatisfiable split constraints
6  Output directory or write failure
7  Output integrity verification failure
8  User interruption or internal error
```

Errors go to standard error and include the cause, relevant path or page range,
and an actionable suggestion where possible.

## Verification

Before publication, every output must:

- Open successfully as a PDF.
- Contain the page count declared by its planned range.
- Participate in a plan that covers every input page exactly once and in order.
- Respect `--max-size`, except for explicitly warned one-page outputs.

Visual-content compatibility is tested by rendering fixture pages before and
after splitting and comparing the rendered images with a small renderer
tolerance.

## Testing Strategy

- Unit tests cover CLI parsing, help behavior, size units, naming, page-count
  balancing, plan invariants, and exit-status mapping.
- Property tests generate random page counts and constraints and assert that
  every plan is continuous, ordered, complete, and non-overlapping.
- Integration fixtures include text, images, rotated pages, mixed page sizes,
  shared resources, and invalid or encrypted PDFs.
- Integration tests perform real splits, reopen outputs, verify counts and
  limits, and compare rendered pages.
- Failure-injection tests simulate write errors, interrupts, and publication
  failures to verify cleanup and overwrite rollback.
- Scale tests cover the target range up to approximately 10,000 pages and 10 GB,
  with explicit memory and runtime observations.
- Release builds and smoke tests cover macOS, Linux, and Windows as standalone
  executables.

## Acceptance Criteria

- Absolute and relative input paths work.
- `--parts` alone creates exactly the requested count with evenly distributed
  page counts.
- `--max-size` alone creates the smallest output count found by the bounded
  adaptive search, balances actual sizes, and strictly observes the limit except
  for warned single pages.
- Combined constraints produce no fewer than `--parts` and use the smallest
  satisfying count found by the bounded adaptive search.
- Every input page appears exactly once, in order, with unchanged visual
  content, dimensions, and rotation.
- Missing output directories are created.
- Conflicts fail by default; overwrite failures restore old files.
- Help, progress, `--no-progress`, warnings, and exit statuses follow this
  specification.
- Processing uses bounded memory at the target scale.
- The project builds as one executable for macOS, Linux, and Windows.

## Non-Goals

- Processing multiple input PDFs in one invocation.
- Reading an input directory.
- Supporting encrypted PDFs or passwords.
- Compressing or reducing page quality to meet a size limit.
- Preserving document-level bookmarks, forms, attachments, metadata, or digital
  signatures.
- Producing a mathematically globally optimal size balance.
