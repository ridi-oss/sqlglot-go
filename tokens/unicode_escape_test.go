package tokens

import "testing"

func TestDecodeUnicodeEscapes(t *testing.T) {
	valid := []struct {
		name string
		in   string
		want string
	}{
		{"no escapes passes through", "information_schema", "information_schema"},
		{"empty", "", ""},
		{"four hex digits", `inf\006Frmation_schema`, "information_schema"},
		{"all four-hex", `\0067\0072\0061\0064\0065`, "grade"},
		{"leading four-hex", `\0067rade`, "grade"},
		{"literal backslash", `a\\b`, `a\b`},
		{"six hex plus form (astral)", `\+01F600`, "\U0001F600"},
		{"six hex plus form (BMP)", `\+000041`, "A"},
		{"surrogate pair 4+4", `\D835\DD0D`, "\U0001D50D"},
		{"surrogate pair 4+6 (mixed width)", `\D835\+00DD0D`, "\U0001D50D"},
		{"surrogate pair 6+4 (mixed width)", `\+00D835\DD0D`, "\U0001D50D"},
		{"surrogate pair 6+6 (mixed width)", `\+00D835\+00DD0D`, "\U0001D50D"},
		{"mixed literal and escape", `x\0079z`, "xyz"},
		{"uppercase hex", `\004F`, "O"},
		{"lowercase hex", `\004f`, "O"},
		{"tab control char is allowed", `a\0009b`, "a\tb"},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			got, ok := decodeUnicodeEscapes(tc.in)
			if !ok {
				t.Fatalf("decodeUnicodeEscapes(%q) reported invalid, want %q", tc.in, tc.want)
			}
			if got != tc.want {
				t.Fatalf("decodeUnicodeEscapes(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// Sequences PostgreSQL rejects must fail closed (ok=false) so the tokenizer raises rather
	// than fabricate a value that diverges from the server. Verified against PostgreSQL 17.6.
	invalid := []struct {
		name string
		in   string
	}{
		{"NUL code point", `\0000`},
		{"out of range", `\+110000`},
		{"lone high surrogate", `\D800`},
		{"lone low surrogate", `\DC00`},
		{"high surrogate then non-surrogate", `\D835\0041`},
		{"high surrogate then nothing", `x\D835`},
		{"incomplete four-hex escape", `a\00`},
		{"non-hex after backslash", `a\g0z`},
		{"non-hex in plus form", `\+00ZZ00`},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if got, ok := decodeUnicodeEscapes(tc.in); ok {
				t.Fatalf("decodeUnicodeEscapes(%q) = %q, ok=true; want fail closed (ok=false)", tc.in, got)
			}
		})
	}
}
