package goreadit

import (
	"io"
	"time"
	"sync"
	"errors"
	"bytes"
	"bufio"
	"io/ioutil"
)

const maxBufferSize = int64(^uint(0) >> 1) // max int

type limitedReader struct {
	R io.Reader // underlying reader
	N int64     // max bytes remaining
}

func (l *limitedReader) Read(p []byte) (n int, err error) {
	if l.N <= 0 {
		return 0, ErrLimitReached
	}
	if int64(len(p)) > l.N {
		p = p[0:l.N]
	}
	n, err = l.R.Read(p)
	l.N -= int64(n)
	return
}

type Reader struct {
	timeout time.Duration
	limit   int64
	reader  io.ReadCloser
	splitFn bufio.SplitFunc
	mu      sync.Mutex

	buf           bytes.Buffer
	limReader     *limitedReader
	scanner       *bufio.Scanner
	readingCh     chan error
	closeCh       chan struct{}
	keepReadingCh chan struct{}

	onceClose, onceRun sync.Once
}

var (
	ErrLimitReached = errors.New("limit is reached")
	ErrTimeout      = errors.New("timeout is reached")
	ErrClosed       = errors.New("reader closed")
)

func NewReader(s io.ReadCloser) *Reader {
	lr := &limitedReader{s, maxBufferSize}
	return &Reader{
		reader:        s,
		limReader:     lr,
		scanner:       bufio.NewScanner(lr),
		closeCh:       make(chan struct{}),
		keepReadingCh: make(chan struct{}, 1),
	}
}

func (r *Reader) fgRead() {
	defer close(r.readingCh)

	var err error

	for {
		select {
		case _, ok := <-r.keepReadingCh:
			if !ok {
				return
			}
		case <-r.closeCh:
			return
		}
		if r.limit > 0 {
			r.limReader.N = r.limit
		} else {
			r.limit = maxBufferSize
		}
		if r.splitFn != nil {
			if r.scanner.Scan() {
				r.buf.Write(r.scanner.Bytes())
			} else {
				err = r.scanner.Err()
				// very strange behavior...
				// https://golang.org/src/bufio/scan.go?s=3814:3843#L80
				if err == nil {
					err = io.EOF
				}
			}
		} else {
			_, err = r.buf.ReadFrom(r.limReader)
		}
		select {
		case <-r.closeCh:
			return
		case r.readingCh <- err:
		}
	}
}

func (r *Reader) ReadBytes() (data [] byte, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	select {
	case <-r.closeCh:
		return nil, ErrClosed
	default:
	}
	var timeoutCh <-chan time.Time
	var ok bool
	r.buf.Reset()

	r.onceRun.Do(func() {
		r.readingCh = make(chan error)
		go r.fgRead()
	})

	if r.timeout > 0 {
		toTimer := time.NewTimer(r.timeout)
		defer toTimer.Stop()

		timeoutCh = toTimer.C
	}

	select {
	case r.keepReadingCh <- struct{}{}:
	default:
	}

	select {
	case <-r.closeCh:
		return nil, ErrClosed
	case <-timeoutCh:
		return nil, ErrTimeout
	case err, ok = <-r.readingCh:
		if !ok {
			err = io.EOF
		}
	}

	data, _ = ioutil.ReadAll(&r.buf)
	return data, err
}

func (r *Reader) ReadString() (s string, err error) {
	b, err := r.ReadBytes()
	return string(b), err
}

func (r *Reader) Close() error {
	go r.onceClose.Do(func() {
		close(r.closeCh)

		r.mu.Lock()
		defer r.mu.Unlock()

		close(r.keepReadingCh)
	})
	return r.reader.Close()
}

func (r *Reader) SetSeparator(sep []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sep == nil {
		r.splitFn = nil
	} else {
		r.splitFn = func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			if i := bytes.Index(data, sep); i >= 0 {
				return i + 1, data[0:i], nil
			}
			// If we're at EOF, we have a final, non-terminated line. Return it.
			if atEOF {
				return len(data), data, nil
			}
			// Request more data.
			return 0, nil, nil
		}
	}
	r.scanner.Split(r.splitFn)
}

func (r *Reader) SetTimeout(to time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.timeout = to
}

func (r *Reader) SetLimit(limit int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.limit = limit
}
