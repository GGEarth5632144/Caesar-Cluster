package controller

import "testing"

func TestMaskGmail(t *testing.T) {
	cases := map[string]string{
		"kittisak@gmail.com": "ki******@gmail.com",
		"abc@g.sut.ac.th":    "ab*@g.sut.ac.th",
		"ab@gmail.com":       "a*@gmail.com",
		"a@gmail.com":        "a@gmail.com",
		"no-at-sign":         "no-at-sign",
	}
	for in, want := range cases {
		if got := maskGmail(in); got != want {
			t.Errorf("maskGmail(%q) = %q, want %q", in, got, want)
		}
	}
}
