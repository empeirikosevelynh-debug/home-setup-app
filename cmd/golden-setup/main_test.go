package main

import (
	"bytes"
	"context"
	"encoding/json"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/testutil"
	"strings"
	"testing"
)

func TestCLIPlan(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWith(context.Background(), []string{"--plan"}, bytes.NewReader(nil), &out, &errOut, func(context.Context, domain.Options) (domain.Host, error) {
		return testutil.FreshHost(t.TempDir()), nil
	})
	var p domain.Plan
	if code != 0 || json.Unmarshal(out.Bytes(), &p) != nil || !p.Supported || p.Accepted {
		t.Fatalf("preview not emitted: %d %s %s", code, out.String(), errOut.String())
	}
}
func TestVersionAndHelpDoNotInspect(t *testing.T) {
	for _, arg := range []string{"--version", "--help"} {
		var out, errs bytes.Buffer
		code := runWith(context.Background(), []string{arg}, nil, &out, &errs, func(context.Context, domain.Options) (domain.Host, error) {
			t.Fatal("metadata inspected the machine")
			return domain.Host{}, nil
		})
		text := out.String() + errs.String()
		if code != 0 || !strings.Contains(text, "Golden Gate Setup") || (arg == "--version" && !strings.Contains(text, "0.1.0-rc.1")) {
			t.Fatalf("unusable metadata: %d %s", code, text)
		}
	}
}
func TestUnknownFlagExit(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runWith(context.Background(), []string{"--force-install"}, nil, &out, &errOut, nil); code != 2 || errOut.Len() == 0 {
		t.Fatal("unknown or unsafe flag did not fail before inspection")
	}
}
