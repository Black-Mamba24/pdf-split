# pdf-split

`pdf-split` is a standalone Go CLI for splitting one PDF into ordered,
continuous page ranges. Every input page appears exactly once and in its
original order.

## Install

From a local checkout, install a native executable into `~/.local/bin`:

```sh
make install
pdf-split --help
```

Override the installation directory when needed:

```sh
make install BINDIR=/usr/local/bin
```

To install the latest remote version without cloning the repository:

```sh
mkdir -p "$HOME/.local/bin"
GOBIN="$HOME/.local/bin" GOARCH="$(go env GOHOSTARCH)" CGO_ENABLED=0 \
  go install github.com/Black-Mamba24/pdf-split/cmd/pdf-split@latest
```

## Usage

```text
pdf-split <input.pdf> [--parts N] [--max-size SIZE] [--output DIR] [--overwrite] [--no-progress]
```

```sh
pdf-split report.pdf --parts 4
pdf-split report.pdf --max-size 10MB --output ./result
pdf-split report.pdf --parts 4 --max-size 10MB --overwrite
```

At least one of `--parts` and `--max-size` is required.

- `--parts N` creates exactly `N` files and measures candidate ranges to make
  their actual sizes as even as practical. With `--max-size`, it sets the
  minimum output count.
- `--max-size SIZE` enforces actual output file sizes. Units are
  case-insensitive binary `KB`, `MB`, and `GB`.
- A single page larger than `--max-size` is emitted alone with a warning.
- `--output DIR` defaults to the current directory and creates missing parents.
- `--overwrite` replaces conflicts only after every new output verifies.
- `--no-progress` disables progress. Interactive terminals use dynamic
  refreshes; redirected stderr and IDE consoles receive retained log lines.

Outputs use the input basename and at least three digits, such as
`report-001.pdf`. Existing conflicts fail unless `--overwrite` is supplied.

## Guarantees And Limits

The tool verifies ordered page coverage, output readability, page counts, and
actual maximum sizes before publishing. Publication uses same-filesystem
staging and restores existing targets if replacement fails.

Page content, dimensions, and rotation are preserved. Document-level features
such as bookmarks, forms, attachments, metadata, signatures, and encryption
are not guaranteed. Encrypted PDFs are unsupported. Version 1 targets inputs
up to approximately 10,000 pages and 10 GB.

## Exit Statuses

| Code | Meaning |
| ---: | --- |
| 0 | Success, including oversized single-page warnings |
| 2 | Invalid CLI arguments |
| 3 | Missing, unreadable, or invalid input PDF |
| 4 | Unsupported PDF, including encryption |
| 5 | Unsatisfiable split constraints |
| 6 | Output directory or write failure |
| 7 | Output integrity verification failure |
| 8 | User interruption or internal error |

## Development

```sh
go test -race ./...
go vet ./...
bash scripts/check-licenses.sh
go test -tags visual ./internal/integration -run TestSplitPagesRenderLikeOriginal -v
```

The visual test requires Poppler's `pdftoppm`. Set `PDF_SPLIT_SCALE_INPUT` to
run the opt-in large-file integration test.
