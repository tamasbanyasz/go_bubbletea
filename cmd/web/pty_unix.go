//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixPTY struct {
	cmd  *exec.Cmd
	ptmx *os.File
}

func startPTY(ctx context.Context, cmdPath string, args []string) (ptyProcess, error) {
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &unixPTY{cmd: cmd, ptmx: ptmx}, nil
}

func (p *unixPTY) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

func (p *unixPTY) Write(buf []byte) (int, error) {
	return p.ptmx.Write(buf)
}

func (p *unixPTY) Close() error {
	return p.ptmx.Close()
}

func (p *unixPTY) Resize(cols, rows int) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixPTY) Wait(ctx context.Context) error {
	return p.cmd.Wait()
}
