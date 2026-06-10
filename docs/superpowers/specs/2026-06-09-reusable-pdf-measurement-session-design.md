# Reusable PDF Measurement Session Design

## Summary

`pdf-split` currently measures every candidate page range by reopening the
input PDF, parsing it, extracting the selected pages, writing a temporary PDF,
and reading the temporary file size. Maximum-size planning measures many
overlapping ranges, so repeated parsing and temporary-file I/O dominate
planning time.

This change introduces an optional reusable measurement session for input PDFs
whose on-disk size is at most 1 GiB. The session parses the source PDF once,
reuses the resulting read-only pdfcpu context for all planning measurements,
and serializes each extracted candidate into a byte-counting writer instead of
a temporary file.

Inputs larger than 1 GiB continue to use the existing low-memory
`WriteRange + stat` measurement path. Engines without session support and
explicitly classified compatibility failures also fall back to the existing
path. Unexpected session setup failures terminate planning. Final output
generation and verification remain unchanged.

## Goals

- Reduce planning time by at least 50% for representative PDFs at most 1 GiB.
- Preserve the user-facing split contract, strict maximum-size guarantees,
  warnings, progress output, and final verification.
- Keep the existing low-memory behavior for PDFs larger than 1 GiB.
- Bound the reusable session lifetime to one planning operation.
- Keep the optimization internal and require no new CLI options.

## Non-Goals

- Reusing the parsed context during final output generation or verification.
- Running candidate measurements concurrently.
- Changing planner search, balancing, cache, or measurement-budget behavior.
- Estimating range sizes without serializing candidate PDFs.
- Dynamically selecting the strategy from available system memory.
- Guaranteeing lower memory use for eligible inputs.

## Current Behavior and Bottleneck

The planner calls `measure.Measurer.Measure` for single pages and candidate
ranges. On a cache miss, the measurer:

1. Creates a temporary output path.
2. Calls `pdf.Engine.WriteRange`.
3. Calls pdfcpu `api.TrimFile`.
4. Reopens and parses the complete input through
   `ReadValidateAndOptimize`.
5. Extracts the requested pages into a new context.
6. Serializes the candidate PDF to disk.
7. Stats and deletes the candidate file.

The LRU cache avoids repeating an identical `(start, end)` measurement, but it
does not help distinct overlapping ranges such as `1-59`, `1-58`, and `1-60`.
For those ranges, the complete source PDF is parsed repeatedly.

## Selected Architecture

### Strategy Selection

Planning selects one measurement strategy before invoking the planner:

| Input condition | Strategy |
| --- | --- |
| Input size `<= 1 GiB` and session opens successfully | Reusable session |
| Input size `> 1 GiB` | Existing file-backed measurer |
| Input cannot be statted | Return an input error |
| Eligible input has a classified compatibility failure while opening a session | Existing file-backed measurer |
| Eligible input has an unexpected I/O or internal failure while opening a session | Fail planning |

The threshold is a package constant expressed as `1 << 30` bytes. Inputs
exactly 1 GiB are eligible.

pdfcpu v0.12.1 mutates the source context while migrating named destinations
during page extraction. Inputs whose parsed context contains a `Dests` name
tree are therefore classified as unsupported for reusable measurement and
automatically use the file-backed measurer.

Fallback is deliberately limited. Falling back after an unexpected error could
hide corruption, permission failures, or defects and would repeat expensive
work without a clear reason.

### Interfaces

The existing `pdf.Engine` interface remains responsible for inspection and
writing final ranges:

```go
type Engine interface {
    Inspect(path string) (Info, error)
    WriteRange(inputPath, outputPath string, pageRange domain.PageRange) error
}
```

The optimization adds optional session capabilities:

```go
type MeasurementSession interface {
    MeasureRange(pageRange domain.PageRange) (int64, error)
    Close() error
}

type MeasurementSessionOpener interface {
    OpenMeasurementSession(inputPath string) (MeasurementSession, error)
}
```

The application checks whether the configured engine implements
`MeasurementSessionOpener`. Tests and alternate engines that only implement
`Engine` continue to use the existing measurer.

`MeasureRange` does not accept `context.Context` because pdfcpu extraction and
serialization do not expose interruption points. The outer measurer checks the
context immediately before every call, matching current behavior. Cancellation
cannot interrupt a measurement already executing.

### pdfcpu Session

`pdfcpuEngine.OpenMeasurementSession`:

1. Opens the input file once.
2. Builds the same fast pdfcpu configuration used by `WriteRange`.
3. Sets the command mode to `TRIM`.
4. calls `api.ReadValidateAndOptimize` once.
5. Rejects parsed contexts containing a `Dests` name tree with
   `ErrMeasurementSessionUnsupported` and closes the source.
6. Retains the source file and parsed source context until `Close`.

`MeasureRange`:

1. Validates that the inclusive one-based range is within the source page
   count.
2. Builds the ordered page-number slice for the range.
3. Calls `pdfcpu.ExtractPages(sourceContext, pageNumbers, false)`.
4. Calls `api.WriteContext(candidateContext, countingWriter)`.
5. Returns the number of serialized bytes.

`pdfcpu.ExtractPages` creates a new destination context and copies selected
pages from the source context. The source context is treated as immutable after
session construction.

The session is not thread-safe. `MeasureRange` calls must remain serial. This
matches the current planner, which performs measurements synchronously.

### Counting Writer

The reusable session serializes candidates into an internal writer that
discards bytes while counting them:

```go
type countingWriter struct {
    bytes int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
    w.bytes += int64(len(p))
    return len(p), nil
}
```

This reports the exact byte count emitted by that specific pdfcpu planning
serialization stream without storing the candidate in memory or writing it to
disk. It is not a heuristic estimate, but it is also not an authoritative final
output size.

Separate pdfcpu serializations of the same range can differ by a few bytes, for
example because object ordering can depend on Go map iteration. Compatibility
comparisons against `WriteRange + stat` are diagnostic only. Final files are
regenerated through `Engine.WriteRange`, statted, and verified independently; if
final bytes exceed the limit, existing verification and replanning remain the
source of truth.

### Measurer Integration

`internal/measure` retains ownership of:

- Context cancellation checks.
- LRU range-size caching.
- Measurement counts.
- Progress start and completion events.
- File-backed candidate cleanup.

It gains a reusable-session-backed range writer. The measurer uses the session
on cache misses when one was selected; otherwise it uses the existing
`Engine.WriteRange + stat` path.

The session is opened before the planner starts and closed immediately after
planning finishes, including error and cancellation paths. It is not retained
during final output generation.

### Data Flow

For eligible inputs:

```text
Inspect input
  -> stat input and select reusable strategy
  -> open and parse source once
  -> planner requests range
  -> LRU cache lookup
  -> extract pages from reusable source context
  -> serialize candidate to counting writer
  -> cache and return planning reference size
  -> close session after planning
  -> generate final outputs through existing WriteRange
  -> stat final files and run existing final verification
```

For inputs larger than 1 GiB:

```text
Inspect input
  -> stat input and select file-backed strategy
  -> planner requests range
  -> existing WriteRange to temporary file
  -> stat, cache, and delete candidate
  -> generate and verify outputs as today
```

## Error Handling and Fallback

Errors are divided into three categories:

- **Unsupported reusable-session capability:** the engine does not implement
  the optional opener. Use the existing measurer.
- **Classified session compatibility error:** pdfcpu reports that the reusable
  path cannot safely process an otherwise supported input. Close partial
  resources and use the existing measurer.
- **Operational or internal error:** file open, read, allocation, extraction,
  serialization, or close failure. Return the error and preserve current exit
  classification.

The initial implementation should define one sentinel,
`pdf.ErrMeasurementSessionUnsupported`, for classified fallback. Encryption and
invalid-PDF errors are not fallback conditions because the existing path cannot
correct them.

The initial classified compatibility case is a parsed pdfcpu context containing
named destinations. Repeated `ExtractPages` calls may patch its source name
tree, so session setup must close the source and return
`pdf.ErrMeasurementSessionUnsupported` before planning starts.

Once planning has successfully started with a reusable session, an individual
`MeasureRange` failure terminates planning. Mid-planning fallback is excluded:
it complicates progress and cache semantics, may hide deterministic failures,
and may repeat substantial work.

`Close` errors are returned only when no earlier planning error exists. If
planning already failed, the planning error remains primary.

## Correctness and Compatibility

The optimization must not weaken the maximum-size contract:

- Planning reference sizes remain based on actual pdfcpu serialization in the
  selected measurement path, not heuristic estimates.
- The same range-size LRU cache is used.
- Planner behavior and measurement budgets are unchanged.
- Final outputs are still regenerated using `Engine.WriteRange`.
- Final outputs are still statted and strictly verified.
- Existing replan-on-final-size-overflow behavior remains unchanged.
- Only final generated file sizes are authoritative for enforcing `--max-size`.

Holding a parsed source context may expose pdfcpu mutation assumptions. Tests
must repeatedly measure overlapping and reordered ranges from one session and
verify that measurements continue to succeed. Measurements stay serial until
pdfcpu explicitly guarantees safe concurrent reads.

## Resource and Memory Management

- The reusable session is eligible only when input size is at most 1 GiB.
- The source file descriptor and source context live only during planning.
- Each candidate context becomes unreachable immediately after its measurement.
- Candidate bytes are discarded by the counting writer.
- No collection of candidate contexts or candidate byte buffers is retained.
- Existing LRU entries retain only range keys and `int64` sizes.
- Inputs larger than 1 GiB retain the existing low-memory behavior.

The implementation must not call `runtime.GC` or tune `GOMEMLIMIT`. Runtime
memory policy remains the caller's responsibility.

## Observability

Existing user-facing progress messages remain unchanged. Strategy selection is
not printed during normal execution.

Benchmarks and tests record the selected strategy explicitly. If diagnostic
logging is added later, it should report:

- Input byte size.
- Selected strategy.
- Session-open duration.
- Planning measurement count and duration.
- Fallback reason.

Diagnostic logging is not part of this change.

## Testing Strategy

### Unit Tests

`internal/pdf`:

- Opening a session parses a valid input and reports correct page bounds.
- Measuring a range returns a positive planning reference size.
- Repeated, overlapping, and reordered measurements continue to succeed.
- Invalid and out-of-bounds ranges fail without corrupting the session.
- Closing releases the input and rejects or safely handles later use.
- Unsupported-session errors are distinguishable from operational failures.
- Compatibility comparisons with `WriteRange + stat` are diagnostic only and do
  not define correctness.

`internal/measure`:

- Session-backed measurements preserve cache, count, progress, and
  cancellation behavior.
- Cache hits do not invoke the session.
- Session measurement failures propagate.
- Existing file-backed tests remain unchanged and passing.

`internal/app`:

- Inputs exactly 1 GiB select the reusable session.
- Inputs one byte larger than 1 GiB select the file-backed measurer.
- Engines without the optional interface use the file-backed measurer.
- Classified session-open failures fall back.
- Unexpected session-open failures stop planning.
- Sessions close after success, planning error, and cancellation.

### Integration Tests

- Run reusable and file-backed strategies on fixtures containing text, images,
  rotations, mixed dimensions, and shared resources, then compare the
  user-facing correctness contract: page coverage, requested minimum output
  count, warnings semantics, readability, and final verification results.
- Record plan and planning-reference-size differences as diagnostics rather than
  pass/fail criteria.
- Verify final outputs continue to satisfy strict size limits.
- Verify no measurement candidate files are created by the reusable strategy.
- Verify the existing strategy still works for a synthetic input classified as
  larger than 1 GiB without requiring a 1 GiB fixture.

### Benchmarks

Add an opt-in planning benchmark using representative external fixtures:

```bash
PDF_SPLIT_BENCH_INPUT=/path/to/input.pdf go test ./internal/integration \
  -run '^$' -bench BenchmarkPlanningStrategies -benchmem
```

The benchmark runs both strategies against the same input and reports:

- Wall-clock planning duration.
- Allocated bytes and allocations.
- Peak resident memory when available from the platform-specific harness.
- Completed measurement count.
- Produced plan, planning reference sizes, and final verification outcome when
  generated.

Benchmark fixtures should include:

- A text-heavy PDF with shared fonts.
- An image-heavy PDF.
- A mixed-content PDF with at least several hundred pages.

## Acceptance Criteria

- For representative input PDFs at most 1 GiB, reusable-session planning is at
  least 50% faster than the current file-backed strategy.
- For the same input and options, both strategies preserve the same user-facing
  correctness contract: valid coverage, requested minimum output count, warning
  semantics, and final verification results. Planning reference sizes and plan
  boundaries may differ and are not authoritative.
- Peak memory for the reusable strategy does not exceed
  `3 * input file size + 512 MiB`.
- Inputs larger than 1 GiB automatically use the existing file-backed strategy
  and show no material regression.
- Final generated outputs and strict size-limit verification remain unchanged.
- All existing unit and integration tests pass.

## Rollout and Risk Control

The reusable strategy becomes the default for eligible inputs after tests and
benchmarks pass. No user-facing flag is added.

Primary risks and controls:

| Risk | Control |
| --- | --- |
| Parsed context consumes excessive memory | 1 GiB eligibility threshold, planning-scoped lifetime, benchmark memory acceptance |
| pdfcpu mutates source context during extraction | Serial measurements, repeated-overlap tests, and explicit compatibility fallback for known mutable structures |
| pdfcpu mutates named destinations during extraction | Detect the `Dests` name tree during session setup and fall back to file-backed measurement |
| Planning reference size differs from final file output | Final stat, verification, and replanning are authoritative; compatibility comparisons are diagnostic only |
| Session setup fails for a compatible PDF | Classified fallback to existing file-backed path |
| Optimization hides real failures | Fallback only for explicit unsupported-session errors |
| Large-input behavior regresses | Inputs larger than 1 GiB never open a reusable session |

## Expected Code Changes

- `internal/pdf/engine.go`
  - Add optional measurement-session interfaces and fallback sentinel.
- `internal/pdf/pdfcpu.go`
  - Implement the reusable pdfcpu session and counting writer.
- `internal/pdf/measurement_session_test.go`
  - Add session lifecycle, bounds, reuse, and planning-reference tests.
- `internal/measure/measurer.go`
  - Allow cache misses to use a reusable session while preserving existing
    file-backed behavior.
- `internal/measure/measurer_test.go`
  - Add session-backed measurer tests.
- `internal/app/app.go`
  - Stat input, select strategy at the 1 GiB threshold, and manage session
    lifetime.
- `internal/app/app_test.go`
  - Add selection, fallback, close, and error-path tests.
- `internal/integration/scale_test.go`
  - Add opt-in comparative performance coverage or host the benchmark beside
    the existing scale test.
