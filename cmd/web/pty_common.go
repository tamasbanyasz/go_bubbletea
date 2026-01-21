package main

import "context"

type ptyProcess interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Resize(cols, rows int) error
	Wait(ctx context.Context) error
}
