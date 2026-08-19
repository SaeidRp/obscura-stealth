package main

import "testing"

func TestResolveSettleWait(t *testing.T) {
	t.Setenv("SIDECAR_DEFAULT_WAIT", "3")
	t.Setenv("SIDECAR_DEFAULT_TEXT_WAIT", "10")

	tests := []struct {
		name      string
		requested int
		dump      string
		want      int
	}{
		{name: "HTML uses the short page settle", dump: "html", want: 3},
		{name: "text uses the longer API settle", dump: "text", want: 10},
		{name: "an explicit wait overrides the dump default", requested: 7, dump: "text", want: 7},
		{name: "wait is bounded by the hard ceiling", requested: maxWaitSeconds + 1, dump: "text", want: maxWaitSeconds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSettleWait(tt.requested, tt.dump); got != tt.want {
				t.Fatalf("resolveSettleWait(%d, %q) = %d, want %d", tt.requested, tt.dump, got, tt.want)
			}
		})
	}
}
