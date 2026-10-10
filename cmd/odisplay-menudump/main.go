// Command odisplay-menudump prints the menu as text.
//
// It builds the same model the menu bar app puts on screen, from the same real
// settings file, and writes it out. That is how the layout gets checked on a
// machine that has no menu bar, and it is what the panel tests read.
//
// It never switches anything. The only call it makes is `list --json`.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/h1st0ry3D/odisplay-mac/internal/cli"
	"github.com/h1st0ry3D/odisplay-mac/internal/panel"
)

func main() {
	client, err := cli.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\n", err)
		fmt.Fprintln(os.Stderr, "the menu would say:")
		fmt.Println(panel.NotInstalled(cli.Candidates()).Describe())
		os.Exit(1)
	}

	settings, err := client.List(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read the settings: %v\n", err)
		os.Exit(1)
	}

	status := panel.Idle
	// A status is only interesting if one is asked for, and the flag exists so
	// the failure wording can be looked at without having to cause one.
	if code := argAfter("-code"); code != "" {
		res := cli.Result{Code: atoi(code)}
		status = panel.ForSwitch(res, "Mac Mini")
	}

	fmt.Printf("display %d, %d inputs\n\n", settings.Display, len(settings.Inputs))
	fmt.Println(panel.Build(settings, status, false).Describe())
}

func argAfter(flag string) string {
	for i, a := range os.Args {
		if a == flag && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
