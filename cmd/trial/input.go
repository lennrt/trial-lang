package main

import (
	"context"
	"io"
	"os"
)

// commandInput interrupts stdin reads when the CLI command is canceled. Closing
// inherited os.Stdin alone does not reliably unblock a pending OS read. A pipe
// lets the command return and finish cleanup independently of that read.
//
// This is only for the CLI: a goroutine blocked on inherited stdin can remain
// until process exit. Library servers retain caller-owned input and lifetimes.
func commandInput(ctx context.Context) (io.Reader, func()) {
	reader, writer := io.Pipe()
	go func() {
		_, err := io.Copy(writer, os.Stdin)
		_ = writer.CloseWithError(err)
	}()
	stop := context.AfterFunc(ctx, func() {
		_ = reader.CloseWithError(ctx.Err())
	})
	return reader, func() {
		stop()
		_ = reader.Close()
	}
}
