package naming

import "testing"

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"hello":            "hello",
		"Hello_World":      "hello-world",
		"./examples/hello": "examples-hello",
		"My App v2!!":      "my-app-v2",
		"---":              "app",
		"":                 "app",
	}

	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
