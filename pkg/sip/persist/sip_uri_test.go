package persist

import "testing"

func TestExtractSIPUserPart(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`"bob" <sip:bob@10.0.4.12>;tag=TahyfH6ZW`, "bob"},
		{`<sip:13800138000@example.com:5060>;tag=abc`, "13800138000"},
		{`sip:test1@10.0.4.12`, "test1"},
		{`sip:alice@host`, "alice"},
		{`tel:+8613800138000`, "+8613800138000"},
		{`"4001608853" <sip:4001608853@gw.example.com>`, "4001608853"},
		{`<sips:secure@example.com>`, "secure"},
		{` `, ""},
		{`bob@example.com`, "bob"},
	}
	for _, tc := range cases {
		got := ExtractSIPUserPart(tc.in)
		if got != tc.want {
			t.Errorf("ExtractSIPUserPart(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
