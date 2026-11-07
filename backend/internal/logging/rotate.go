package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

const (
	defaultMaxBytes   int64 = 100 << 20
	defaultMaxBackups       = 30
)

// rollingFile writes a single log stream to <dir>/<prefix>.log and rotates it
// when the date changes or the file would exceed maxBytes. Rotated files are
// compressed with zstd in the background, and only the newest keep archives
// are retained.
type rollingFile struct {
	dir    string
	prefix string
	max    int64
	keep   int
	now    func() time.Time

	mu   sync.Mutex
	file *os.File
	day  string
	size int64

	queueMu sync.Mutex
	queue   []string
	closed  bool
	wake    *sync.Cond
	done    chan struct{}
}

func newRollingFile(dir, prefix string, maxBytes int64, keep int) (*rollingFile, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if keep <= 0 {
		keep = defaultMaxBackups
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &rollingFile{
		dir:    dir,
		prefix: prefix,
		max:    maxBytes,
		keep:   keep,
		now:    time.Now,
		done:   make(chan struct{}),
	}
	w.wake = sync.NewCond(&w.queueMu)
	if err := w.open(); err != nil {
		return nil, err
	}
	go w.compressLoop()
	w.enqueueStaleArchives()
	return w, nil
}

func (w *rollingFile) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	now := w.now()
	today := now.Format("2006-01-02")
	switch {
	case w.size == 0:
		// Nothing to rotate; just adopt the current day.
		w.day = today
	case w.day != today || w.size+int64(len(p)) > w.max:
		if err := w.rotate(now); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rollingFile) Close() error {
	w.mu.Lock()
	var err error
	if w.file != nil {
		err = w.file.Close()
		w.file = nil
	}
	w.mu.Unlock()

	w.queueMu.Lock()
	w.closed = true
	w.wake.Broadcast()
	w.queueMu.Unlock()
	<-w.done
	return err
}

func (w *rollingFile) open() error {
	name := filepath.Join(w.dir, w.prefix+".log")
	file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	w.day = w.now().Format("2006-01-02")
	if w.size > 0 {
		// An existing file keeps the day it was last written to, so a restart
		// across midnight still splits the archive by content day.
		w.day = info.ModTime().Format("2006-01-02")
	}
	return nil
}

func (w *rollingFile) rotate(now time.Time) error {
	active := filepath.Join(w.dir, w.prefix+".log")
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	archived := w.archiveName(w.day)
	if err := os.Rename(active, archived); err != nil {
		if !os.IsNotExist(err) {
			// Keep logging to the original file if it cannot be moved.
			if openErr := w.open(); openErr != nil {
				return err
			}
			return err
		}
	} else {
		w.enqueue(archived)
	}
	return w.open()
}

// archiveName builds a unique name for a rotated file. The day identifies the
// content, and a counter separates multiple rotations within that day.
func (w *rollingFile) archiveName(day string) string {
	if day == "" {
		day = w.now().Format("2006-01-02")
	}
	base := fmt.Sprintf("%s-%s", w.prefix, day)
	name := filepath.Join(w.dir, base+".log")
	for i := 1; ; i++ {
		if !fileExists(name) && !fileExists(name+".zst") {
			return name
		}
		name = filepath.Join(w.dir, fmt.Sprintf("%s-%02d.log", base, i))
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (w *rollingFile) enqueue(path string) {
	w.queueMu.Lock()
	w.queue = append(w.queue, path)
	w.wake.Signal()
	w.queueMu.Unlock()
}

func (w *rollingFile) enqueueStaleArchives() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, w.prefix+"-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		w.enqueue(filepath.Join(w.dir, name))
	}
}

func (w *rollingFile) compressLoop() {
	defer close(w.done)
	for {
		w.queueMu.Lock()
		for len(w.queue) == 0 && !w.closed {
			w.wake.Wait()
		}
		if len(w.queue) == 0 && w.closed {
			w.queueMu.Unlock()
			return
		}
		batch := w.queue
		w.queue = nil
		w.queueMu.Unlock()

		for _, path := range batch {
			if err := w.compressFile(path); err != nil {
				fmt.Fprintf(os.Stderr, "logging: compress %s: %v\n", path, err)
				continue
			}
			_ = os.Remove(path)
		}
		w.prune()
	}
}

// compressFile streams path into path+".zst" and leaves the source in place on
// failure. Its encoder lives only for the duration of the call: zstd keeps
// match tables worth several MB, and an idle server should not hold them.
func (w *rollingFile) compressFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	outPath := path + ".zst"
	tmp := outPath + ".tmp"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(dst,
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(1<<20),
		zstd.WithLowerEncoderMem(true),
	)
	if err != nil {
		dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := io.Copy(encoder, src); err != nil {
		encoder.Close()
		dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := encoder.Close(); err != nil {
		dst.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, outPath)
}

func (w *rollingFile) prune() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	active := w.prefix + ".log"
	type archive struct {
		name string
		mod  time.Time
	}
	var archives []archive
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == active || !strings.HasPrefix(name, w.prefix+"-") {
			continue
		}
		if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.zst") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		archives = append(archives, archive{name: name, mod: info.ModTime()})
	}
	if len(archives) <= w.keep {
		return
	}
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].mod.After(archives[j].mod)
	})
	for _, item := range archives[w.keep:] {
		_ = os.Remove(filepath.Join(w.dir, item.name))
	}
}
