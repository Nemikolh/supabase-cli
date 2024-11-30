package term

import (
	"io"
	"os"
	"syscall/js"
)

// terminalState holds the platform-specific state / console mode for the terminal.
type terminalState struct {
	mode uint32
}

func stdStreams() (stdIn io.ReadCloser, stdOut, stdErr io.Writer) {
	return os.Stdin, os.Stdout, os.Stderr
}

func setRawTerminal(fd uintptr) (previousState *State, err error) {
	return makeRaw(fd)
}

func isTerminal(fd uintptr) bool {
	globalThis := js.Global()
	ttyMod := globalThis.Call("require", "node:tty")
	return ttyMod.Call("isatty", fd).Bool()
}

func setRawTerminalOutput(fd uintptr) (previousState *State, err error) {
	return nil, nil
}

// getFdInfo returns the file descriptor for an os.File and indicates whether the file represents a terminal.
func getFdInfo(in interface{}) (uintptr, bool) {
	var inFd uintptr
	var isTerminalIn bool
	if file, ok := in.(*os.File); ok {
		inFd = file.Fd()
		isTerminalIn = isTerminal(inFd)
	}
	return inFd, isTerminalIn
}

func getWinsize(fd uintptr) (*Winsize, error) {
	globalThis := js.Global()
	ttyMod := globalThis.Call("require", "node:tty")

	writeStream := ttyMod.Get("WriteStream").New(fd)

	sizes := writeStream.Call("getWindowSize")
	numColumns := sizes.Index(0).Int()
	numRows := sizes.Index(1).Int()

	return &Winsize{
		Height: uint16(numRows),
		Width:  uint16(numColumns),
	}, nil
}

func setWinsize(fd uintptr, ws *Winsize) error {
	return nil
}

func restoreTerminal(fd uintptr, state *State) error {
	return nil
}

func saveState(fd uintptr) (*State, error) {
	return nil, nil
}

func disableEcho(fd uintptr, state *State) error {
	return nil
}
