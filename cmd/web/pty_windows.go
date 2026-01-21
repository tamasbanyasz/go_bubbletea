//go:build windows

package main

import (
	"context"
	"strings"

	"github.com/UserExistsError/conpty"
)

type windowsPTY struct {
	cpty *conpty.ConPty
}

func startPTY(ctx context.Context, cmdPath string, args []string) (ptyProcess, error) {
	cmdLine := windowsCommandLine(cmdPath, args)
	cpty, err := conpty.Start(cmdLine)
	if err != nil {
		return nil, err
	}
	return &windowsPTY{cpty: cpty}, nil
}

func (p *windowsPTY) Read(buf []byte) (int, error) {
	return p.cpty.Read(buf)
}

func (p *windowsPTY) Write(buf []byte) (int, error) {
	return p.cpty.Write(buf)
}

func (p *windowsPTY) Close() error {
	return p.cpty.Close()
}

func (p *windowsPTY) Resize(cols, rows int) error {
	return p.cpty.Resize(cols, rows)
}

func (p *windowsPTY) Wait(ctx context.Context) error {
	_, err := p.cpty.Wait(ctx)
	return err
}

func windowsCommandLine(cmdPath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, windowsQuote(cmdPath))
	for _, arg := range args {
		parts = append(parts, windowsQuote(arg))
	}
	return strings.Join(parts, " ")
}

func windowsQuote(value string) string {
	if value == "" {
		return "\"\""
	}
	if !strings.ContainsAny(value, " \t\"") {
		return value
	}
	replacer := strings.NewReplacer(`"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}
