package logger

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func retrieveFileContents(path string) (string, error) {
	var (
		file     *os.File
		err      error
		sc       *bufio.Scanner
		contents strings.Builder
	)
	file, err = os.Open(path)
	defer file.Close()
	sc = bufio.NewScanner(file)

	for sc.Scan() {
		contents.WriteString(sc.Text() + "\n")
	}
	if err = sc.Err(); err != nil {
		return "", err
	}

	return contents.String(), nil
}

func TestInit(t *testing.T) {
	var (
		tempDir  string = t.TempDir()
		tempFile string = filepath.Join(tempDir, "test.log")
		err      error
	)

	if err = Init(tempFile); os.IsNotExist(err) {
		t.Fatal("Log file wasn't initialized as expected")
	}
}

func TestInitFail(t *testing.T) {
	var (
		tempDir string = t.TempDir()
		err     error
	)

	if err = Init(tempDir); err == nil {
		t.Fatal("Fail didn't occurred as expected")
	}
}

func TestInfo(t *testing.T) {
	var (
		tempDir  string = t.TempDir()
		tempFile string = filepath.Join(tempDir, "test.log")
		content  string
		err      error
	)

	_ = Init(tempFile)
	Info("Hello")

	content, err = retrieveFileContents(tempFile)
	if err != nil {
		t.Fatalf("Failed to read the log contents: %v", err.Error())
	}

	if !strings.Contains(content, "[INFO]Hello") {
		t.Error("Info() didn't log the correct message")
	}
}

func TestWarn(t *testing.T) {
	var (
		tempDir  string = t.TempDir()
		tempFile string = filepath.Join(tempDir, "test.log")
		content  string
		err      error
	)

	_ = Init(tempFile)
	Warn("Do not panic")

	content, err = retrieveFileContents(tempFile)
	if err != nil {
		t.Fatalf("Failed to read the log contents: %v", err.Error())
	}

	if !strings.Contains(content, "[Warn]Do not panic") {
		t.Error("Info() didn't log the correct message")
	}
}

func TestError(t *testing.T) {
	var (
		tempDir  string = t.TempDir()
		tempFile string = filepath.Join(tempDir, "test.log")
		content  string
		err      error
	)

	_ = Init(tempFile)
	Error("You died")

	content, err = retrieveFileContents(tempFile)
	if err != nil {
		t.Fatalf("Failed to read the log contents: %v", err.Error())
	}

	if !strings.Contains(content, "[ERROR]You died") {
		t.Error("Info() didn't log the correct message")
	}
}
