package expand

import "testing"

/*
TestExpandInterpolateRunCommandFoldsMultiline verifies that the line wrapping introduced by
triple-quoted strings — including newlines coming from an interpolated multiline global — collapses
into a single clean command line, while deliberate horizontal spacing between arguments is preserved.
*/
func TestExpandInterpolateRunCommandFoldsMultiline(t *testing.T) {
	t.Parallel()

	globals := map[string]string{
		"FLAGS": "\n\t-g -std=c89\n\t-Wall -Werror=vla\n\t-Wno-long-long\n",
	}

	cases := []struct {
		name    string
		literal string
		want    string
	}{
		{
			name:    "multiline global interpolated into single-line run template",
			literal: "clang main.c ${FLAGS} -o out",
			want:    "clang main.c -g -std=c89 -Wall -Werror=vla -Wno-long-long -o out",
		},
		{
			name:    "triple-quoted run body with leading and trailing newlines",
			literal: "\n\tclang main.c\n\t-g -std=c89\n\t-o out\n",
			want:    "clang main.c -g -std=c89 -o out",
		},
		{
			name:    "deliberate multi-space alignment without line breaks is preserved",
			literal: "cp  src   dst",
			want:    "cp  src   dst",
		},
		{
			name:    "carriage-return line endings fold too",
			literal: "echo a\r\n  echo b",
			want:    "echo a echo b",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExpandInterpolateRunCommand(tc.literal, globals, nil)
			if got != tc.want {
				t.Fatalf("ExpandInterpolateRunCommand(%q) = %q, want %q", tc.literal, got, tc.want)
			}
		})
	}
}
