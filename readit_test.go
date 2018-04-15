package goreadit

import (
	"testing"
	"time"
	"strings"
	"io/ioutil"
	"io"
)

type stupidReader struct {
	base      *strings.Reader
	readDelay time.Duration
}

func (r *stupidReader) Read(p []byte) (n int, err error) {
	time.Sleep(r.readDelay)
	return r.base.Read(p)
}

func newStupidReader(str string, readDelay time.Duration) io.ReadCloser {
	return ioutil.NopCloser(&stupidReader{
		base:      strings.NewReader(str),
		readDelay: readDelay,
	})
}

func TestReader_ReadBytes(t *testing.T) {
	const controlString = "Test string"

	// Test limits
	r := NewReader(newStupidReader(controlString, 0))
	r.SetLimit(4)

	str, err := r.ReadString()
	if err != ErrLimitReached {
		t.Errorf("Expect: %v, got: %v", ErrLimitReached, err)
	}
	if str != controlString[:4] {
		t.Errorf("Expect: %v, got: %v", controlString[:4], str)
	}

	// Test timeouts
	r = NewReader(newStupidReader(controlString, 2*time.Second))
	r.SetTimeout(1 * time.Second)

	str, err = r.ReadString()
	if err != ErrTimeout {
		t.Errorf("Expect: %v, got: %v", ErrTimeout, err)
	}
	if str != "" {
		t.Errorf("Expect: %v, got: %v", "", str)
	}

	// Test closing
	r = NewReader(newStupidReader(controlString, 2*time.Second))
	time.AfterFunc(1*time.Second, func() { r.Close() })

	str, err = r.ReadString()
	if err != ErrClosed {
		t.Errorf("Expect: %v, got: %v", ErrClosed, err)
	}
	if str != "" {
		t.Errorf("Expect: %v, got: %v", "", str)
	}

	// Test separator
	r = NewReader(newStupidReader(controlString, 0))
	r.SetSeparator([]byte(" "))

	str, err = r.ReadString()
	if err != nil {
		t.Errorf("Expect: %v, got: %v", nil, err)
	}
	if str != "Test" {
		t.Errorf("Expect: %v, got: %v", "Test", str)
	}

	str, err = r.ReadString()
	if err != nil {
		t.Errorf("Expect: %v, got: %v", nil, err)
	}
	if str != "string" {
		t.Errorf("Expect: %v, got: %v", "string", str)
	}

	str, err = r.ReadString()
	if err != io.EOF {
		t.Errorf("Expect: %v, got: %v", io.EOF, err)
	}
	if str != "" {
		t.Errorf("Expect: %v, got: %v", "string", str)
	}
}
