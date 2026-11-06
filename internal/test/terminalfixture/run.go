// Package terminalfixture runs reviewed native CLI demonstrations in an isolated pseudo-terminal.
package terminalfixture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// Step waits for a visible prompt before supplying one answer or an explicit interruption.
type Step struct {
	Prompt    string `json:"prompt"`
	Answer    string `json:"answer,omitempty"`
	Interrupt bool   `json:"interrupt,omitempty"`
	EOF       bool   `json:"eof,omitempty"`
}

// Result separates machine output from the real terminal conversation and process status.
type Result struct {
	Stdout      string `json:"stdout"`
	Transcript  string `json:"transcript"`
	ExitCode    int    `json:"exitCode"`
	AnswersSent int    `json:"answersSent"`
}

// Terminal is what the child's terminal declares through TERM and NO_COLOR. The fixture sets both from it in place of
// any values in the child's environment, so a developer's terminal can't change what a test sees. The zero value
// declares no terminal type and no color preference, as in CI.
type Terminal struct {
	Type    string
	NoColor bool
}

// Run is Terminal{}.Run.
func Run(ctx context.Context, binary, directory string, args []string, steps []Step) (Result, error) {
	return Terminal{}.Run(ctx, binary, directory, args, steps)
}

// RunWithEnvironment is Terminal{}.RunWithEnvironment.
func RunWithEnvironment(ctx context.Context, binary, directory string, environment, args []string, steps []Step) (Result, error) {
	return Terminal{}.RunWithEnvironment(ctx, binary, directory, environment, args, steps)
}

// Run owns a child terminal, bounded output, and cancellation, without executing a shell or changing parent streams.
// The child inherits the test's environment with a PATH that holds no executables.
func (terminal Terminal) Run(ctx context.Context, binary, directory string, args []string, steps []Step) (Result, error) {
	return terminal.RunWithEnvironment(ctx, binary, directory, append(os.Environ(), "PATH="+directory+"/no-runtime"), args, steps)
}

// RunWithEnvironment is Run with the given child environment, such as one that puts Git on PATH, apart from the
// terminal's settings.
func (terminal Terminal) RunWithEnvironment(ctx context.Context, binary, directory string, environment, args []string, steps []Step) (Result, error) {
	if len(steps) > 20 {
		return Result{}, fmt.Errorf("at most 20 prompt answers are allowed")
	}
	for _, step := range steps {
		if step.Prompt == "" || len(step.Answer) > 4096 || strings.ContainsAny(step.Answer, "\r\n\x00") {
			return Result{}, fmt.Errorf("each step needs a prompt and one bounded answer line")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	master, slave, err := pty.Open()
	if err != nil {
		return Result{}, err
	}
	defer master.Close()
	defer slave.Close()
	if err := unix.SetNonblock(int(master.Fd()), true); err != nil {
		return Result{}, err
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = directory
	command.Env = terminal.environment(environment)
	command.Stdin, command.Stderr = slave, slave
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = time.Second
	if err := command.Start(); err != nil {
		return Result{}, err
	}
	// Keep the parent's slave open until capture has drained the master: macOS can discard
	// unread output when the last slave descriptor closes, which loses a quick child's output.
	// See https://github.com/fabricahq/trailguide/pull/65.
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(exited)
	}()
	var transcript strings.Builder
	sent, offset := 0, 0
	readErr := capture(ctx, master, command, exited, steps, &transcript, &sent, &offset)
	if readErr != nil {
		cancel()
		drain(master, exited)
	}
	<-exited
	result := Result{Stdout: stdout.String(), Transcript: transcript.String(), AnswersSent: sent}
	if readErr != nil {
		return result, readErr
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if waitErr != nil {
		var exit *exec.ExitError
		if !errors.As(waitErr, &exit) {
			return result, waitErr
		}
		result.ExitCode = exit.ExitCode()
	}
	if sent != len(steps) {
		return result, fmt.Errorf("process ended before prompt %q", steps[sent].Prompt)
	}
	return result, nil
}

// environment returns base with its TERM and NO_COLOR replaced by the terminal's.
func (terminal Terminal) environment(base []string) []string {
	environment := slices.DeleteFunc(slices.Clone(base), func(item string) bool {
		key, _, _ := strings.Cut(item, "=")
		return key == "TERM" || key == "NO_COLOR"
	})
	if terminal.Type != "" {
		environment = append(environment, "TERM="+terminal.Type)
	}
	if terminal.NoColor {
		environment = append(environment, "NO_COLOR=1")
	}
	return environment
}

// capture drains terminal bytes and sends a step only after its expected prompt appears.
// It ends once a poll that began after the process exited finds no more output.
func capture(ctx context.Context, master *os.File, command *exec.Cmd, exited <-chan struct{}, steps []Step, transcript *strings.Builder, sent, offset *int) error {
	fd := int(master.Fd())
	buffer := make([]byte, 4096)
	var pending []byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		finished := false
		select {
		case <-exited:
			finished, pending = true, nil
		default:
		}
		ready := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if len(pending) > 0 {
			ready[0].Events |= unix.POLLOUT
		}
		n, err := unix.Poll(ready, 100)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			if finished {
				return nil
			}
			continue
		}
		if ready[0].Revents&unix.POLLOUT != 0 && len(pending) > 0 {
			written, writeErr := unix.Write(fd, pending[:min(len(pending), 256)])
			if written > 0 {
				pending = pending[written:]
			}
			if writeErr != nil && writeErr != unix.EAGAIN && writeErr != unix.EINTR {
				return writeErr
			}
		}
		if ready[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
			continue
		}
		n, err = unix.Read(fd, buffer)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if err == unix.EIO || n == 0 && err == nil {
			return nil
		}
		if err != nil {
			return err
		}
		if transcript.Len()+n > 2*1024*1024 {
			return fmt.Errorf("terminal output exceeds 2 MiB")
		}
		transcript.Write(buffer[:n])
		if *sent >= len(steps) {
			continue
		}
		step := steps[*sent]
		index := strings.Index(transcript.String()[*offset:], step.Prompt)
		if index < 0 {
			continue
		}
		*offset += index + len(step.Prompt)
		*sent++
		switch {
		case step.Interrupt:
			err = command.Process.Signal(os.Interrupt)
		case step.EOF:
			pending = []byte{4}
		default:
			pending = []byte(step.Answer + "\r")
		}
		if err != nil {
			return err
		}
	}
}

// drain discards terminal output until the process exits. On macOS a process can't finish exiting while its
// terminal output waits for a reader, so a stopped capture must keep reading or the process never ends.
func drain(master *os.File, exited <-chan struct{}) {
	fd := int(master.Fd())
	buffer := make([]byte, 4096)
	for {
		select {
		case <-exited:
			return
		default:
		}
		ready := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if n, err := unix.Poll(ready, 100); err != nil && err != unix.EINTR || n > 0 && ready[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) != 0 {
			if _, err := unix.Read(fd, buffer); err != nil && err != unix.EAGAIN && err != unix.EINTR {
				// The terminal is gone, so nothing can block the exit any longer.
				<-exited
				return
			}
		}
	}
}
