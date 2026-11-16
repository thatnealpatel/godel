package godel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchFreshStdin(t *testing.T) {
	if mode := os.Getenv("GODEL_LAUNCH_STDIN_TEST"); mode != "" {
		if mode == "closed" {
			// Close the owning File too, so its
			// finalizer cannot later close the
			// temporary file that reuses fd 0.
			if err := os.Stdin.Close(); err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal(launchFresh(context.Background(), os.Getenv("GODEL_LAUNCH_SOURCE"), strings.Repeat("fresh stdin\x00\n", 100000)))
		return
	}

	// Exercise the real go run exec without
	// involving the runner lifecycle.
	source := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module launchfixture\n\ngo 1.23\n",
		"main.go": `package main
import (
    "io"
    "os"
)
func main() {
    if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
        panic(err)
    }
}
`,
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"open", "closed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLaunchFreshStdin$")
			cmd.Env = append(os.Environ(), "GODEL_LAUNCH_STDIN_TEST="+mode, "GODEL_LAUNCH_SOURCE="+source)
			cmd.WaitDelay = 2 * time.Second
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("launch with %s stdin: %v\n%s", mode, err, output)
			}
			// More than pipe capacity, binary data, and EOF
			// must all survive exec (including clearing CLOEXEC
			// on reused fd 0).
			want := strings.Repeat("fresh stdin\x00\n", 100000)
			if string(output) != want {
				t.Fatalf("exec received incorrect stdin: got %d bytes, want %d", len(output), len(want))
			}
		})
	}
}
