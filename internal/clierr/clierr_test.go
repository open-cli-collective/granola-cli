package clierr

import (
	"errors"
	"testing"
)

func TestCodeOfNilIsZero(t *testing.T) {
	if got := CodeOf(nil); got != 0 {
		t.Fatalf("CodeOf(nil) = %d, want 0", got)
	}
}

func TestCodeOfDefaultsToGeneric(t *testing.T) {
	if got := CodeOf(errors.New("boom")); got != int(CodeGeneric) {
		t.Fatalf("CodeOf(plain error) = %d, want %d", got, CodeGeneric)
	}
}

func TestCodeOfFindsWrappedTypedError(t *testing.T) {
	err := Wrap(CodeAuthConfig, errors.New("missing token"), "status unhealthy")
	if got := CodeOf(err); got != int(CodeAuthConfig) {
		t.Fatalf("CodeOf(wrapped) = %d, want %d", got, CodeAuthConfig)
	}
}

func TestHasExitCode(t *testing.T) {
	if HasExitCode(errors.New("boom")) {
		t.Fatal("HasExitCode(plain error) = true, want false")
	}
	if !HasExitCode(New(CodeUsage, "bad input")) {
		t.Fatal("HasExitCode(typed error) = false, want true")
	}
}

func TestLooksLikeUsage(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		extra []string
		want  bool
	}{
		{name: "default pattern", err: errors.New("unknown flag: --wat"), want: true},
		{name: "extra pattern", err: errors.New("stdin is not a TTY; provide --username"), extra: []string{"stdin is not a TTY; provide "}, want: true},
		{name: "plain error", err: errors.New("boom"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLikeUsage(tc.err, tc.extra...); got != tc.want {
				t.Fatalf("LooksLikeUsage(%v, %v) = %v, want %v", tc.err, tc.extra, got, tc.want)
			}
		})
	}
}
