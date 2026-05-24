package ghc

import "testing"

func TestParseRepoArg(t *testing.T) {
	tests := []struct {
		in        string
		wantOwner string
		wantName  string
		wantErr   bool
	}{
		{in: "cjunks94/nitpick", wantOwner: "cjunks94", wantName: "nitpick"},
		{in: "owner/name/extra", wantOwner: "owner", wantName: "name/extra"}, // SplitN keeps the rest
		{in: "nitpick", wantErr: true},                                       // no slash — was the original panic
		{in: "/nitpick", wantErr: true},                                      // empty owner
		{in: "cjunks94/", wantErr: true},                                     // empty name
		{in: "", wantErr: true},
		{in: "/", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			owner, name, err := ParseRepoArg(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got owner=%q name=%q", owner, name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner || name != tt.wantName {
				t.Errorf("got (%q, %q), want (%q, %q)", owner, name, tt.wantOwner, tt.wantName)
			}
		})
	}
}
