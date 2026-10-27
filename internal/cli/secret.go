package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// readSecret reads one line from in: at a terminal with a prompt on out and
// without echo, otherwise (a pipe or a file) as it comes.
func readSecret(in io.Reader, out io.Writer, prompt string) (string, error) {
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		_, _ = fmt.Fprint(out, prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		_, _ = fmt.Fprintln(out)
		return string(b), err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
