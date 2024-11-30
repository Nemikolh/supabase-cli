package term

import (
	"syscall/js"
)

func makeRaw(fd uintptr) (*State, error) {
	globalThis := js.Global()
	ttyMod := globalThis.Call("require", "node:tty")

	readStream := ttyMod.Get("ReadStream").New(fd)
	readStream.Call("setRawMode", true)

	return nil, nil
}
