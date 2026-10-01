package downloader

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
)

type OutputStream string

const (
	StreamStdout OutputStream = "stdout"
	StreamStderr OutputStream = "stderr"
)

type OutputLine struct {
	Stream OutputStream `json:"stream"`
	Text   string       `json:"text"`
}

type LineHandler func(OutputLine)

type ProcessError struct {
	Program    string
	ExitCode   int
	StderrTail []string
	Err        error
}

func (e *ProcessError) Error() string {
	message := fmt.Sprintf("%s 실행에 실패했습니다", e.Program)
	if e.ExitCode >= 0 {
		message += fmt.Sprintf(" (exit code %d)", e.ExitCode)
	}
	if len(e.StderrTail) > 0 {
		message += ": " + strings.Join(e.StderrTail, " | ")
	}
	return message
}

func (e *ProcessError) Unwrap() error { return e.Err }

type Runner struct{}

func NewRunner() *Runner { return &Runner{} }

func (r *Runner) Run(ctx context.Context, spec CommandSpec, handler LineHandler) error {
	if strings.TrimSpace(spec.Path) == "" {
		return fmt.Errorf("실행할 프로그램 경로가 비어 있습니다")
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s 실행이 취소되었습니다: %w", filepath.Base(spec.Path), err)
	}

	cmd := exec.Command(spec.Path, spec.Args...)
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout 파이프를 만들 수 없습니다: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr 파이프를 만들 수 없습니다: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s 실행을 시작할 수 없습니다: %w", filepath.Base(spec.Path), err)
	}

	stopCancelWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = terminateProcessTree(cmd)
		case <-stopCancelWatch:
		}
	}()

	lines := make(chan OutputLine, 64)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go scanOutput(stdout, StreamStdout, lines, errs, &wg)
	go scanOutput(stderr, StreamStderr, lines, errs, &wg)
	go func() {
		wg.Wait()
		close(lines)
		close(errs)
	}()

	stderrTail := make([]string, 0, 12)
	for line := range lines {
		if line.Stream == StreamStderr && strings.TrimSpace(line.Text) != "" {
			stderrTail = appendTail(stderrTail, line.Text, 12)
		}
		if handler != nil {
			handler(line)
		}
	}

	var scanErr error
	for err := range errs {
		if err != nil && scanErr == nil {
			scanErr = err
		}
	}

	waitErr := cmd.Wait()
	close(stopCancelWatch)
	if ctx.Err() != nil {
		return fmt.Errorf("%s 실행이 취소되었습니다: %w", filepath.Base(spec.Path), ctx.Err())
	}
	if scanErr != nil {
		return fmt.Errorf("%s 출력을 읽는 중 오류가 발생했습니다: %w", filepath.Base(spec.Path), scanErr)
	}
	if waitErr != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return &ProcessError{
			Program:    filepath.Base(spec.Path),
			ExitCode:   exitCode,
			StderrTail: stderrTail,
			Err:        waitErr,
		}
	}
	return nil
}

func scanOutput(reader io.Reader, stream OutputStream, lines chan<- OutputLine, errs chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines <- OutputLine{Stream: stream, Text: scanner.Text()}
	}
	if err := scanner.Err(); err != nil {
		errs <- err
	}
}

func appendTail(lines []string, line string, max int) []string {
	if max <= 0 {
		return lines[:0]
	}
	if len(lines) == max {
		copy(lines, lines[1:])
		lines[len(lines)-1] = line
		return lines
	}
	return append(lines, line)
}


func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if goruntime.GOOS == "windows" {
		treeKill := exec.Command(
			"taskkill.exe",
			"/PID",
			strconv.Itoa(cmd.Process.Pid),
			"/T",
			"/F",
		)
		treeKill.Stdout = io.Discard
		treeKill.Stderr = io.Discard
		if err := treeKill.Run(); err == nil {
			return nil
		}
	}

	return cmd.Process.Kill()
}
