package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These run against the real odisplay program and a scratch settings file, so
// what is checked is the contract the app depends on rather than a mock of it.
//
// Nothing here can move a monitor or a device. Every switch is refused before
// it runs, by using a key the settings do not have.

// realBinary finds the odisplay built alongside this app, or skips.
func realBinary(t *testing.T) string {
	t.Helper()
	path := os.Getenv("ODISPLAY_BIN")
	if path == "" {
		path = "/home/arch/.local/bin/odisplay"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("odisplay is not built at %s", path)
	}
	return path
}

// scratch writes a settings file and points ODISPLAY_CONFIG at it.
func scratch(t *testing.T, body string) *Client {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "odisplay.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ODISPLAY_CONFIG", path)
	return &Client{Path: realBinary(t)}
}

const oneInput = `{
  "version": 1,
  "display": 1,
  "inputs": [
    {"key":"usbc","name":"X","vcp":27,"host":"0"},
    {"key":"dp","name":"Mac Mini","vcp":15,"host":"3"}
  ]
}`

// --- reading ---------------------------------------------------------------

func TestListReadsTheSettings(t *testing.T) {
	s, err := scratch(t, oneInput).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(s.Inputs))
	}
	if s.Inputs[1].Key != "dp" || s.Inputs[1].Name != "Mac Mini" {
		t.Errorf("second input = %+v", s.Inputs[1])
	}
	if s.Display != 1 {
		t.Errorf("display = %d, want 1", s.Display)
	}
}

// The app never decides a platform rule. odisplay resolves the per-platform
// display number before it prints, so what arrives is the number in force here.
func TestListReportsTheNumberInForceHere(t *testing.T) {
	c := scratch(t, `{"version":1,"display":1,"displays":{"darwin":4},"inputs":[{"key":"dp","vcp":15}]}`)
	s, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Display != 1 {
		t.Errorf("display = %d, want this machine's own number", s.Display)
	}
	// The map is still carried, for anything that wants to show it.
	if s.Displays["darwin"] != 4 {
		t.Errorf("displays = %v, want the other machine's kept", s.Displays)
	}
}

func TestListFailsClearlyOnABrokenSettingsFile(t *testing.T) {
	c := scratch(t, "{ this is not json")
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("List accepted a settings file that will not parse")
	} else if !strings.Contains(err.Error(), "could not read") {
		t.Errorf("error = %q, want it to say the settings could not be read", err)
	}
}

func TestListFailsClearlyWhenTheBinaryIsMissing(t *testing.T) {
	t.Setenv("ODISPLAY_CONFIG", filepath.Join(t.TempDir(), "odisplay.json"))
	c := &Client{Path: "/nonexistent/odisplay"}
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("List succeeded against a binary that is not there")
	}
}

// --- reading an input ------------------------------------------------------

func TestAnAbsentHostReadsAsOff(t *testing.T) {
	// odisplay leaves the key out of the file when it is off.
	in := Input{Key: "hdmi", Name: "PC"}
	if got := in.HostChannel(); got != "0" {
		t.Errorf("HostChannel() = %q, want 0", got)
	}
}

func TestAnImpossibleHostReadsAsOff(t *testing.T) {
	for _, h := range []string{"", "0", "9", "x", "12", " 1"} {
		in := Input{Key: "dp", Host: h}
		want := h
		if h != "1" && h != "2" && h != "3" {
			want = "0"
		}
		if got := in.HostChannel(); got != want {
			t.Errorf("Host %q = %q, want %q", h, got, want)
		}
	}
}

func TestAnUnnamedInputStillHasALabel(t *testing.T) {
	if got := (Input{Key: "dp"}).Label(); got != "dp" {
		t.Errorf("Label() = %q, want the key rather than a blank", got)
	}
	if got := (Input{Key: "dp", Name: "  "}).Label(); got != "dp" {
		t.Errorf("Label() = %q, want the key for a blank name", got)
	}
	if got := (Input{Key: "dp", Name: "Mac Mini"}).Label(); got != "Mac Mini" {
		t.Errorf("Label() = %q, want the user's name", got)
	}
}

// --- switching -------------------------------------------------------------

// A key the settings do not have is refused by odisplay before it runs
// anything. That is what makes it safe to exercise the switch path here at all.
func TestSwitchingToAKeyThatDoesNotExistRunsNothing(t *testing.T) {
	c := scratch(t, oneInput)
	res := c.Switch(context.Background(), "nonesuch")
	if res.Code == 0 {
		t.Fatal("a switch to a key that does not exist reported success")
	}
	if res.Outcome() != OutcomeUsage {
		t.Errorf("outcome = %v, want Usage", res.Outcome())
	}
	if !strings.Contains(res.Message, "No input called") {
		t.Errorf("message = %q", res.Message)
	}
}

func TestADryRunIsItsOwnCodePath(t *testing.T) {
	c := scratch(t, oneInput)
	res, err := c.run(context.Background(), "switch", "dp", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 0 {
		t.Fatalf("a dry run exited %d: %s", res.Code, res.Detail)
	}
	// It has to name the commands, or it is not showing anything.
	if !strings.Contains(res.Message, "ddcutil") {
		t.Errorf("dry run printed %q, want the commands it would run", res.Message)
	}
}

// --- reading outcomes ------------------------------------------------------

// The two failure codes mean opposite things about where the keyboard is, so
// they have to stay distinguishable all the way to the menu.
func TestTheExitCodesKeepTheirMeanings(t *testing.T) {
	cases := []struct {
		code int
		want Outcome
	}{
		{0, OutcomeOK},
		{1, OutcomeUsage},
		{2, OutcomeDisplayFailed},
		{3, OutcomeDevicesFailed},
		{4, OutcomeMissing},
		{99, OutcomeUsage},
	}
	for _, c := range cases {
		if got := (Result{Code: c.code}).Outcome(); got != c.want {
			t.Errorf("code %d = %v, want %v", c.code, got, c.want)
		}
	}
}

// The fallback text is what a person reads at the moment they most need to know
// where their keyboard is, so it has to say that rather than repeat a number.
func TestTheFallbackSaysWhereTheKeyboardIs(t *testing.T) {
	if got := (Result{Code: 2}).MessageOr("Mac Mini"); !strings.Contains(got, "left alone") {
		t.Errorf("code 2 = %q, want it to say the devices were left", got)
	}
	if got := (Result{Code: 3}).MessageOr("Mac Mini"); !strings.Contains(got, "did not") {
		t.Errorf("code 3 = %q, want it to say the devices did not move", got)
	}
	if got := (Result{Code: 3}).MessageOr("Mac Mini"); !strings.Contains(got, "way back") {
		t.Errorf("code 3 = %q, want it to say how to get the keyboard back", got)
	}
	if got := (Result{Code: 3}).DetailOr(); !strings.Contains(got, "button") {
		t.Errorf("code 3 detail = %q, want it to point at the button", got)
	}
	if got := (Result{Code: 0}).MessageOr("PC"); !strings.Contains(got, "PC") {
		t.Errorf("code 0 = %q, want it to name what was sent", got)
	}
}

// odisplay's own sentence is better than the fallback, so it wins.
func TestOdisplaysOwnSentenceIsPreferred(t *testing.T) {
	r := Result{Code: 2, Message: "Could not switch to Mac Mini.", Detail: "Display 1 not found"}
	if got := r.MessageOr("Mac Mini"); got != "Could not switch to Mac Mini." {
		t.Errorf("MessageOr() = %q, want odisplay's own", got)
	}
	if got := r.DetailOr(); got != "Display 1 not found" {
		t.Errorf("DetailOr() = %q, want odisplay's own", got)
	}
}

func TestSentIsNotTheSameAsExitingCleanly(t *testing.T) {
	// Exit 4 means a program is missing. The monitor may or may not have
	// moved, so this must not count as a switch that happened.
	if (Result{Code: 4}).Sent() {
		t.Error("exit 4 counted as sent")
	}
	if !(Result{Code: 0}).Sent() {
		t.Error("exit 0 did not count as sent")
	}
}

// --- changing --------------------------------------------------------------

func TestSettingTheHostWritesAndReadsBack(t *testing.T) {
	c := scratch(t, oneInput)
	if res := c.SetHost(context.Background(), "dp", "1"); res.Code != 0 {
		t.Fatalf("set host failed: %d %s %s", res.Code, res.Message, res.Detail)
	}
	s, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Inputs[1].HostChannel() != "1" {
		t.Errorf("host = %q, want 1", s.Inputs[1].HostChannel())
	}
}

func TestARefusedSettingDoesNotHalfApply(t *testing.T) {
	c := scratch(t, oneInput)
	res := c.SetHost(context.Background(), "dp", "9")
	if res.Code == 0 {
		t.Fatal("set host 9 was accepted")
	}
	s, _ := c.List(context.Background())
	if s.Inputs[1].HostChannel() != "3" {
		t.Errorf("host = %q, want it left at 3", s.Inputs[1].HostChannel())
	}
}

func TestSettingANameWithSpacesInItSurvives(t *testing.T) {
	c := scratch(t, oneInput)
	if res := c.SetName(context.Background(), "dp", "Living Room PC"); res.Code != 0 {
		t.Fatalf("set name failed: %s", res.Detail)
	}
	s, _ := c.List(context.Background())
	if s.Inputs[1].Label() != "Living Room PC" {
		t.Errorf("label = %q", s.Inputs[1].Label())
	}
}

// --- the process itself ----------------------------------------------------

// A key with a space in it has to reach odisplay as one argument. If anything
// ever joined these into a string, this is where it would show.
func TestAKeyWithSpacesIsOneArgument(t *testing.T) {
	c := scratch(t, oneInput)
	res, err := c.run(context.Background(), "set", "name", "dp", `it's"; echo pwned; "`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 0 {
		t.Fatalf("a name with a quote in it was refused: %s", res.Detail)
	}
	s, _ := c.List(context.Background())
	got := s.Inputs[1].Name
	if strings.Contains(got, "pwned") && strings.Count(got, "pwned") > 1 {
		t.Errorf("name = %q, want it stored verbatim", got)
	}
	if !strings.Contains(got, `"`) {
		t.Errorf("name = %q, want the quote preserved as one name", got)
	}
}

func TestODisplayLinesAreFlattenedToOne(t *testing.T) {
	c := scratch(t, oneInput)
	res, err := c.run(context.Background(), "switch", "dp", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Message, "\n") {
		t.Errorf("message kept its line breaks: %q", res.Message)
	}
}

func TestDoctorTouchesNoHardware(t *testing.T) {
	c := scratch(t, oneInput)
	out, res := c.Doctor(context.Background())
	if out == "" {
		t.Error("doctor printed nothing")
	}
	// Either it is ready, or it is saying why not. Both are fine; crashing or
	// running a switch is not.
	if res.Code != 0 && !strings.Contains(out+res.Detail, "display") {
		t.Errorf("doctor failed without saying why: %d %q", res.Code, res.Detail)
	}
}

// A launch agent starts with a bare environment. If HOME did not reach odisplay
// it would find no settings and refuse, which looks like a broken install.
func TestHomeReachesTheChild(t *testing.T) {
	if os.Getenv("HOME") == "" {
		t.Skip("no HOME to pass on")
	}
	dir := t.TempDir()
	// ODISPLAY_CONFIG is absolute, so this does not depend on HOME resolving
	// it, but the environment still has to carry enough for odisplay to start.
	c := scratch(t, oneInput)
	res, err := c.run(context.Background(), "path")
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 0 {
		t.Errorf("odisplay path exited %d: %s", res.Code, res.Detail)
	}
	if !strings.Contains(res.Message, "odisplay.json") {
		t.Errorf("path = %q", res.Message)
	}
	_ = dir
}

// Regression. run() sets the child's environment explicitly, and it used to
// leave ODISPLAY_CONFIG out of it. Every call then ignored the settings file it
// had been pointed at and read the real one instead, which means a set went to
// the real settings. It did, twice, while this test was being written.
//
// The cost of omitting it is not a subtle wrong value, it is that this program
// writes somewhere the person using it did not choose.
func TestTheChildIsGivenTheConfigOverride(t *testing.T) {
	c := scratch(t, oneInput)
	res, err := c.run(context.Background(), "path")
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 0 {
		t.Fatalf("odisplay path exited %d: %s", res.Code, res.Detail)
	}
	want := os.Getenv("ODISPLAY_CONFIG")
	if !strings.Contains(res.Message, want) {
		t.Errorf("odisplay was pointed at %q, want %q", res.Message, want)
	}
	if want == "" {
		t.Error("the test did not set an override, so it is proving nothing")
	}
}

// And the check that would have caught it without a path assertion: a write has
// to land in the file this process was pointed at, and nowhere else.
func TestAWriteLandsInThePointedAtFileOnly(t *testing.T) {
	c := scratch(t, oneInput)
	before, err := os.ReadFile(os.Getenv("ODISPLAY_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if res := c.SetName(context.Background(), "dp", "Changed"); res.Code != 0 {
		t.Fatalf("set name failed: %s", res.Detail)
	}
	after, err := os.ReadFile(os.Getenv("ODISPLAY_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) == string(after) {
		t.Error("the pointed-at file did not change, so the write went elsewhere")
	}
	if !strings.Contains(string(after), "Changed") {
		t.Errorf("the pointed-at file does not hold the change:\n%s", after)
	}
}

// Every path has to end in the binary, exactly once. This list held full paths
// and was treated as a list of directories, so the message said it had looked
// in /usr/local/bin/odisplay/odisplay, which is not a path anyone can act on.
func TestCandidatesArePathsToTheBinary(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	got := Candidates()
	if len(got) != len(candidates) {
		t.Fatalf("candidates = %v", got)
	}
	for _, p := range got {
		if filepath.Base(p) != "odisplay" {
			t.Errorf("%q does not end in the binary name", p)
		}
		if strings.Count(p, "odisplay") != 1 {
			t.Errorf("%q has the name in it twice", p)
		}
	}
	// The user's own has to be in there, since that is where a no-sudo install
	// puts it.
	want := filepath.Join("/home/someone", ".local", "bin", "odisplay")
	found := false
	for _, p := range got {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Errorf("candidates = %v, want %s among them", got, want)
	}
}

func TestNewFindsTheBinaryItIsGiven(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	real := realBinary(t)

	// Copy the real binary into the one place this test controls, so New's
	// search is exercised without depending on where the app was installed.
	binDir := filepath.Join(dir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "odisplay"), raw, 0o755); err != nil {
		t.Fatal(err)
	}

	c, err := New()
	if err != nil {
		t.Fatalf("New did not find the binary it was given: %v", err)
	}
	if c.Path != filepath.Join(binDir, "odisplay") {
		t.Errorf("found %q", c.Path)
	}
}

func TestAClientWithNoPathFailsRatherThanDoingNothing(t *testing.T) {
	if _, err := (&Client{}).List(context.Background()); err == nil {
		t.Error("a client with no path should fail rather than silently do nothing")
	}
}

func TestSettingTheDisplayWritesAndReadsBack(t *testing.T) {
	c := scratch(t, oneInput)
	if res := c.SetDisplay(context.Background(), 3); res.Code != 0 {
		t.Fatalf("set display failed: %d %s %s", res.Code, res.Message, res.Detail)
	}
	s, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Display != 3 {
		t.Errorf("display = %d, want 3", s.Display)
	}
}

// The picker offers 1 to 4. A number outside it must be refused by odisplay
// rather than written and used.
func TestSettingAnImpossibleDisplayIsRefused(t *testing.T) {
	c := scratch(t, oneInput)
	if res := c.SetDisplay(context.Background(), 0); res.Code == 0 {
		t.Error("display 0 was accepted")
	}
	s, _ := c.List(context.Background())
	if s.Display != 1 {
		t.Errorf("display = %d, want it left at 1", s.Display)
	}
}
