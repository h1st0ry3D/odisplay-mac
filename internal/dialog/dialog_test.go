package dialog

import (
	"strings"
	"testing"
)

// The AppleScript is the one part of this package that can break in a way
// nothing here would notice, because there is no Mac to run it on. So the
// quoting is checked directly, against names that come from a file a person
// edits by hand.

func TestLiteralQuotesAQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{`plain`, `"plain"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{`back\slash`, `"back\\slash"`},
		{`both "and" \`, `"both \"and\" \\"`},
		{``, `""`},
	}
	for _, c := range cases {
		if got := literal(c.in); got != c.want {
			t.Errorf("literal(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

// An AppleScript string literal cannot span lines, so a newline in a name
// would end the literal early and leave the rest as script.
func TestLiteralCannotBeBrokenOutOf(t *testing.T) {
	for _, in := range []string{
		"two\nlines",
		"carriage\rreturn",
		"tab\there",
		"\"); do shell script \"rm -rf ~\"; --",
	} {
		got := literal(in)
		if strings.ContainsAny(got[1:len(got)-1], "\n\r\t") {
			t.Errorf("literal(%q) = %s, which spans a line", in, got)
		}
		if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
			t.Errorf("literal(%q) = %s, want a quoted literal", in, got)
		}
		// Every quote inside must be escaped, or the literal ends early.
		inner := got[1 : len(got)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '"' && (i == 0 || inner[i-1] != '\\') {
				t.Errorf("literal(%q) = %s, has an unescaped quote", in, got)
				break
			}
		}
	}
}

func TestLiteralKeepsTheWordsApart(t *testing.T) {
	// Dropping the newline would glue the words together into one nobody
	// wrote; a space keeps the sentence readable in the dialog.
	if got := literal("two\nwords"); !strings.Contains(got, "two words") {
		t.Errorf("literal = %s, want the words kept apart", got)
	}
}

// The script has to be one line, because it goes across as a single -e
// argument.
func TestScriptIsOneLine(t *testing.T) {
	got := Script("Name for this input", "current name")
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("Script spans a line: %q", got)
	}
	for _, want := range []string{"display dialog", "default answer", "text returned of"} {
		if !strings.Contains(got, want) {
			t.Errorf("Script is missing %q: %s", want, got)
		}
	}
}

// A name from the settings file must not be able to add to the script. This is
// the case that matters: the file is hand-editable, so its contents are input
// rather than constants.
func TestANameCannotAddToTheScript(t *testing.T) {
	hostile := `x" default button "Cancel" & (do shell script "touch /tmp/pwned") & "`
	got := Script("Name", hostile)

	// The whole hostile string has to sit inside one literal, quoted.
	if !strings.Contains(got, `\"`) {
		t.Errorf("the quote in the name was not escaped: %s", got)
	}
	// And the script still has exactly one display dialog and one default
	// answer, both from this code rather than from the name.
	if n := strings.Count(got, "display dialog"); n != 1 {
		t.Errorf("display dialog appears %d times, want 1: %s", n, got)
	}
	if n := strings.Count(got, "default answer"); n != 1 {
		t.Errorf("default answer appears %d times, want 1: %s", n, got)
	}
}

func TestCancelIsToldApartFromAFailure(t *testing.T) {
	// osascript exits non-zero for both, so what it said is the only signal.
	for _, said := range []string{
		"execution error: User canceled. (-128)",
		"User cancelled.",
	} {
		if !isCancelled(said) {
			t.Errorf("isCancelled(%q) = false, want true", said)
		}
	}
	for _, said := range []string{
		"execution error: Expected end of line but found unknown token. (-2741)",
		"syntax error",
		"",
	} {
		if isCancelled(said) {
			t.Errorf("isCancelled(%q) = true, want false", said)
		}
	}
}

// Asking for a name that is already a name has to give it back unchanged, so
// the dialog opens on what is there rather than on an empty field.
func TestScriptCarriesTheCurrentName(t *testing.T) {
	got := Script("Name", "Living Room PC")
	if !strings.Contains(got, `default answer "Living Room PC"`) {
		t.Errorf("Script does not carry the current name: %s", got)
	}
}
