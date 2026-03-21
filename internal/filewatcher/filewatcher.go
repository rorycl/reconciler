// package filewatcher watches for writes to files matching the specified directory to
// watch + file suffix matching patterns provided as a map[string][]string. Note that
// empty suffixes is an error.
//
// filewatcher uses github.com/fsnotify/fsnotify. This monitors directories for changes,
// the file events of which should be filtered to only include the files of interest.
package filewatcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/sync/errgroup"
)

// defaultFlushDuration sets the time given to wait for multiple editor writes.
const defaultFlushDuration time.Duration = 25 * time.Millisecond

// FileChangeNotifier is a type holding one or more FileChangeDescriptor
// watchers. An update event is an error chan. An empty error is a normal update.
type FileChangeNotifier struct {
	dirDescriptorMap map[string][]string
	watcher          *fsnotify.Watcher
	updateChan       chan error
	flushDuration    time.Duration
}

// NewFileChangeNotifier registers a FileChangeNotifier with a set of directories to
// file suffix maps.
//
// Note that suffixes provided without the leading "dot" ('.') have this
// prepended to the provided suffix.
//
// Refer to
// https://github.com/fsnotify/fsnotify/blob/v1.8.0/cmd/fsnotify/file.go
func NewFileChangeNotifier(ctx context.Context, descriptors map[string][]string) (*FileChangeNotifier, error) {

	if len(descriptors) < 1 {
		return nil, fmt.Errorf("at least one dir/filematch suffix descriptor needed")
	}
	for k, v := range descriptors {
		if len(v) < 1 {
			return nil, fmt.Errorf("descriptor %q had no suffixes defined", k)
		}
	}

	fcn := &FileChangeNotifier{
		dirDescriptorMap: map[string][]string{},
		updateChan:       make(chan error),
		flushDuration:    defaultFlushDuration,
	}

	var err error
	fcn.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify new watcher error: %w", err)
	}

	for rawDir, suffixes := range descriptors {
		dir := filepath.Clean(rawDir)
		check, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("dir %q not found: %w", dir, err)
		}
		if !check.IsDir() {
			return nil, fmt.Errorf("%q is not a directory", dir)
		}
		if _, found := fcn.dirDescriptorMap[dir]; found {
			return nil, fmt.Errorf("%q already registered", dir)
		}
		err = fcn.watcher.Add(dir)
		if err != nil {
			return nil, fmt.Errorf("fsnotify add error for dir %q: %w", dir, err)
		}

		// add the suffixes, prepending "." if necessary.
		fcn.dirDescriptorMap[dir] = []string{}
		for _, ix := range suffixes {
			if len(ix) == 0 {
				return nil, fmt.Errorf("nil length suffix provided for dir %q", dir)
			}
			ix = "." + strings.TrimLeft(ix, ".")
			fcn.dirDescriptorMap[dir] = append(fcn.dirDescriptorMap[dir], ix)
		}
	}

	// Launch the watcher.
	go func() {
		fcn.watch(ctx)
	}()

	return fcn, err
}

// watch watches the filesystem for the registered events, returning any
// error found while doing so. Watch blocks, so needs to be run in a
// goroutine.
//
// watch watches the specified directories for write events for files
// with the specified suffixes. Consumers should iterate over [Update]
// to receive notice of a file write event requiring a refresh.
func (fcn *FileChangeNotifier) watch(ctx context.Context) {

	// eventChan is an internal chan used for buffering editor writes.
	eventChan := make(chan bool)

	g, ctx := errgroup.WithContext(ctx)

	// This goroutine watches for *fsnotify.Watcher events.
	g.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err, ok := <-fcn.watcher.Errors:
				if !ok {
					return errors.New("unexpected close from watcher.Errors")
				}
				return fmt.Errorf("unexpected notify error: %w", err)

			// An event has been received.
			case e, ok := <-fcn.watcher.Events:
				if !ok {
					return errors.New("unexpected close from watcher.Events")
				}
				// skip events that aren't writes
				if !e.Has(fsnotify.Write) {
					continue
				}
				dir := filepath.Dir(e.Name)
				basename := filepath.Base(e.Name)
				// fmt.Printf("event for %s\n    string: %s\n", e.Name, e.String())

				// ignore dot files
				if len(basename) > 0 && basename[0] == '.' {
					continue
				}

				// check the suffixes for this directory
				suffixes, ok := fcn.dirDescriptorMap[dir]
				if !ok {
					return fmt.Errorf("could not find matcher for dir %q", dir)
				}
				for _, ix := range suffixes {
					if strings.HasSuffix(strings.ToLower(basename), strings.ToLower(ix)) {
						eventChan <- true
					}
				}
			}
		}
	})

	// Simple buffer of double writes by editors like vim. This
	// goroutine will exit if the context is Done or eventChan is
	// closed.
	g.Go(func() error {
		flush := false
		timer := time.NewTicker(fcn.flushDuration)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()

			// Stack writes in the same flushDuration, giving time for
			// the writes to complete.
			case _, ok := <-eventChan:
				if !ok {
					return nil
				}
				flush = true
				timer.Reset(fcn.flushDuration)
			case <-timer.C:
				if flush {
					fcn.updateChan <- nil
					flush = false
				}
			}
		}
	})

	fcn.updateChan <- g.Wait()
	close(eventChan)
	close(fcn.updateChan)
	_ = fcn.watcher.Close()
}

// Update returns a channel signalling a file refresh event.
func (fcn *FileChangeNotifier) Update() <-chan error {
	return fcn.updateChan
}
