package output

import (
	"reflect"
	"testing"
)

func TestNamesUseInputBaseAndAtLeastThreeDigits(t *testing.T) {
	got := Names("/tmp/report.final.pdf", 3)
	want := []string{"report.final-001.pdf", "report.final-002.pdf", "report.final-003.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %#v, want %#v", got, want)
	}
}

func TestNamesExpandWidth(t *testing.T) {
	got := Names("report.pdf", 10000)
	if len(got) != 10000 {
		t.Fatalf("Names() returned %d names, want 10000", len(got))
	}
	if got[0] != "report-00001.pdf" {
		t.Fatalf("first name = %q, want report-00001.pdf", got[0])
	}
	if got[len(got)-1] != "report-10000.pdf" {
		t.Fatalf("last name = %q, want report-10000.pdf", got[len(got)-1])
	}
}

func TestNamesPreserveNonPDFExtension(t *testing.T) {
	got := Names("archive.v1", 1)
	want := []string{"archive-001.v1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %#v, want %#v", got, want)
	}
}

func TestNamesRejectNonPositiveCount(t *testing.T) {
	if got := Names("report.pdf", 0); got != nil {
		t.Fatalf("Names() = %#v, want nil", got)
	}
}
