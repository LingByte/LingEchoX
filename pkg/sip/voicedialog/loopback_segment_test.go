package voicedialog

import "testing"

func TestFindSentenceCut(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		runeMin int
		want    int // byte index (use len of expected head)
		head    string
	}{
		{
			name:    "empty",
			s:       "",
			runeMin: 6,
			want:    0,
		},
		{
			name:    "no_punctuation",
			s:       "您好世界",
			runeMin: 6,
			want:    0,
		},
		{
			name:    "strong_chinese_period",
			s:       "您好。世界",
			runeMin: 6,
			head:    "您好。",
		},
		{
			name:    "strong_question_mark",
			s:       "在吗？",
			runeMin: 6,
			head:    "在吗？",
		},
		{
			name:    "strong_ascii_question",
			s:       "ok?",
			runeMin: 6,
			head:    "ok?",
		},
		{
			name:    "soft_comma_below_threshold",
			s:       "好，",
			runeMin: 6,
			want:    0,
		},
		{
			name:    "soft_comma_below_runemin_no_cut",
			s:       "您好请问，下文",
			runeMin: 6,
			want:    0, // comma is the 5th rune; 5 < 6 → no cut
		},
		{
			name:    "soft_comma_at_threshold",
			s:       "您好请问下，文",
			runeMin: 6,
			head:    "您好请问下，",
		},
		{
			name:    "soft_dunhao_at_threshold",
			s:       "苹果香蕉橙子、葡萄",
			runeMin: 6,
			head:    "苹果香蕉橙子、",
		},
		{
			name:    "newline_strong",
			s:       "abc\ndef",
			runeMin: 6,
			head:    "abc\n",
		},
		{
			name:    "decimal_dot_below_threshold",
			s:       "3.14",
			runeMin: 6,
			want:    0,
		},
		{
			name:    "decimal_dot_above_threshold",
			s:       "数值是七八九3.14159 进一步",
			runeMin: 6,
			head:    "数值是七八九3.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findSentenceCut(tc.s, tc.runeMin)
			want := tc.want
			if tc.head != "" {
				want = len(tc.head)
			}
			if got != want {
				t.Fatalf("findSentenceCut(%q,%d) = %d, want %d (head=%q got_head=%q)",
					tc.s, tc.runeMin, got, want, tc.head, tc.s[:got])
			}
		})
	}
}
