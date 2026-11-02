package torutil

import (
	"testing"
)

func TestPartitionString(t *testing.T) {
	assert := func(str string, ch byte, expectedA string, expectedB string, expectedOk bool) {
		t.Helper()
		a, b, ok := PartitionString(str, ch)
		if a != expectedA {
			t.Fatalf("PartitionString(%q, %q) first part: got %q, want %q", str, ch, a, expectedA)
		}
		if b != expectedB {
			t.Fatalf("PartitionString(%q, %q) second part: got %q, want %q", str, ch, b, expectedB)
		}
		if ok != expectedOk {
			t.Fatalf("PartitionString(%q, %q) separator found: got %v, want %v", str, ch, ok, expectedOk)
		}
	}
	assert("foo:bar", ':', "foo", "bar", true)
	assert(":bar", ':', "", "bar", true)
	assert("foo:", ':', "foo", "", true)
	assert("foo", ':', "foo", "", false)
	assert("foo:bar:baz", ':', "foo", "bar:baz", true)
}

func TestPartitionStringFromEnd(t *testing.T) {
	assert := func(str string, ch byte, expectedA string, expectedB string, expectedOk bool) {
		t.Helper()
		a, b, ok := PartitionStringFromEnd(str, ch)
		if a != expectedA {
			t.Fatalf("PartitionStringFromEnd(%q, %q) first part: got %q, want %q", str, ch, a, expectedA)
		}
		if b != expectedB {
			t.Fatalf("PartitionStringFromEnd(%q, %q) second part: got %q, want %q", str, ch, b, expectedB)
		}
		if ok != expectedOk {
			t.Fatalf("PartitionStringFromEnd(%q, %q) separator found: got %v, want %v", str, ch, ok, expectedOk)
		}
	}
	assert("foo:bar", ':', "foo", "bar", true)
	assert(":bar", ':', "", "bar", true)
	assert("foo:", ':', "foo", "", true)
	assert("foo", ':', "foo", "", false)
	assert("foo:bar:baz", ':', "foo:bar", "baz", true)
}

func TestEscapeSimpleQuotedStringIfNeeded(t *testing.T) {
	assert := func(str string, shouldBeDiff bool) {
		t.Helper()
		maybeEscaped := EscapeSimpleQuotedStringIfNeeded(str)
		if shouldBeDiff {
			if maybeEscaped == str {
				t.Fatalf("EscapeSimpleQuotedStringIfNeeded(%q): got %q, want a value different from %q", str, maybeEscaped, str)
			}
		} else {
			if maybeEscaped != str {
				t.Fatalf("EscapeSimpleQuotedStringIfNeeded(%q): got %q, want %q", str, maybeEscaped, str)
			}
		}
	}
	assert("foo", false)
	assert(" foo", true)
	assert("f\\oo", true)
	assert("fo\"o", true)
	assert("f\roo", true)
	assert("fo\no", true)
}

func TestEscapeSimpleQuotedString(t *testing.T) {
	if got := EscapeSimpleQuotedString("foo"); got != "\"foo\"" {
		t.Fatalf("EscapeSimpleQuotedString(%q): got %q, want %q", "foo", got, "\"foo\"")
	}
}

func TestEscapeSimpleQuotedStringContents(t *testing.T) {
	assert := func(str string, expected string) {
		t.Helper()
		if got := EscapeSimpleQuotedStringContents(str); got != expected {
			t.Fatalf("EscapeSimpleQuotedStringContents(%q): got %q, want %q", str, got, expected)
		}
	}
	assert("foo", "foo")
	assert("f\\oo", "f\\\\oo")
	assert("f\\noo", "f\\\\noo")
	assert("f\n o\ro", "f\\n o\\ro")
	assert("fo\r\\\"o", "fo\\r\\\\\\\"o")
}

func TestUnescapeSimpleQuotedStringIfNeeded(t *testing.T) {
	assert := func(str string, expectedStr string, expectedErr bool) {
		t.Helper()
		actualStr, actualErr := UnescapeSimpleQuotedStringIfNeeded(str)
		if actualStr != expectedStr {
			t.Fatalf("UnescapeSimpleQuotedStringIfNeeded(%q): got %q, want %q", str, actualStr, expectedStr)
		}
		if (actualErr != nil) != expectedErr {
			t.Fatalf("UnescapeSimpleQuotedStringIfNeeded(%q) error presence: got %v (error %v), want %v", str, actualErr != nil, actualErr, expectedErr)
		}
	}
	assert("foo", "foo", false)
	assert("\"foo\"", "foo", false)
	assert("\"f\"oo\"", "", true)
}

func TestUnescapeSimpleQuotedString(t *testing.T) {
	assert := func(str string, expectedStr string, expectedErr bool) {
		t.Helper()
		actualStr, actualErr := UnescapeSimpleQuotedString(str)
		if actualStr != expectedStr {
			t.Fatalf("UnescapeSimpleQuotedString(%q): got %q, want %q", str, actualStr, expectedStr)
		}
		if (actualErr != nil) != expectedErr {
			t.Fatalf("UnescapeSimpleQuotedString(%q) error presence: got %v (error %v), want %v", str, actualErr != nil, actualErr, expectedErr)
		}
	}
	assert("foo", "", true)
	assert("\"foo\"", "foo", false)
	assert("\"f\"oo\"", "", true)
}

func TestUnescapeSimpleQuotedStringContents(t *testing.T) {
	assert := func(str string, expectedStr string, expectedErr bool) {
		t.Helper()
		actualStr, actualErr := UnescapeSimpleQuotedStringContents(str)
		if actualStr != expectedStr {
			t.Fatalf("UnescapeSimpleQuotedStringContents(%q): got %q, want %q", str, actualStr, expectedStr)
		}
		if (actualErr != nil) != expectedErr {
			t.Fatalf("UnescapeSimpleQuotedStringContents(%q) error presence: got %v (error %v), want %v", str, actualErr != nil, actualErr, expectedErr)
		}
	}
	assert("foo", "foo", false)
	assert("f\\\\oo", "f\\oo", false)
	assert("f\\\\noo", "f\\noo", false)
	assert("f\\n o\\ro", "f\n o\ro", false)
	assert("fo\\r\\\\\\\"o", "fo\r\\\"o", false)
	assert("f\"oo", "", true)
	assert("f\roo", "", true)
	assert("f\noo", "", true)
	assert("f\\oo", "", true)
}
