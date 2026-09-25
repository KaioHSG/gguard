package engine

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	w      *fsnotify.Watcher
	events chan string
	errs   chan error
}

func NewWatcher(path string) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := w.Add(path); err != nil {
		w.Close()
		return nil, err
	}

	filepath.Walk(path, func(sub string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("watcher: skipping %s: %v", sub, err)
			return nil
		}
		if info.IsDir() && sub != path {
			if err := w.Add(sub); err != nil {
				log.Printf("watcher: cannot watch subdir %s: %v", sub, err)
			}
		}
		return nil
	})

	return &Watcher{
		w:      w,
		events: make(chan string, 100),
		errs:   make(chan error, 1),
	}, nil
}

func (w *Watcher) Start(ctx context.Context) {
	defer close(w.events)
	defer close(w.errs)
	defer w.w.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-w.w.Events:
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				select {
				case w.events <- event.Name:
				case <-ctx.Done():
					return
				}
			}
		case err := <-w.w.Errors:
			select {
			case w.errs <- err:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *Watcher) Events() <-chan string {
	return w.events
}

func (w *Watcher) Errors() <-chan error {
	return w.errs
}