# PDF test fixtures

`basic.pdf` contains five image-backed pages. `encrypted.pdf` is the same PDF
encrypted with user password `test` and owner password `owner`.

Regenerate both fixtures from the repository root:

```sh
go run ./internal/pdf/testdata/generate
```

The generator uses only deterministic standard-library image drawing and
pdfcpu's import and encryption APIs. The generated PDFs may contain differing
PDF metadata between runs, but their page content and encryption settings are
reproducible.
