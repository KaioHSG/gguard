package engine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gguard/gguard/ggs"
)

type Engine struct {
	guard      *ggs.Guard
	vars       map[string]string
	watcher    *Watcher
	debouncer  *Debouncer
	cancelFunc context.CancelFunc
	cacheTimer *time.Timer
	mu         sync.Mutex
}

func New(guard *ggs.Guard, configVars map[string]string) (*Engine, error) {
	vars := make(map[string]string)
	for k, v := range configVars {
		vars[k] = v
	}
	vars["watch"] = resolveVars(guard.Watch, vars)

	watchPath := vars["watch"]
	if _, err := os.Stat(watchPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("watch path does not exist: %s", watchPath)
	}

	w, err := NewWatcher(watchPath)
	if err != nil {
		return nil, fmt.Errorf("cannot create watcher for %s: %w", watchPath, err)
	}

	return &Engine{
		guard:   guard,
		vars:    vars,
		watcher: w,
		debouncer: NewDebouncer(guard.Debounce),
	}, nil
}

func (e *Engine) Start(ctx context.Context) {
	ctx, e.cancelFunc = context.WithCancel(ctx)

	e.debouncer.OnFire(func(file string) {
		vars := make(map[string]string)
		for k, v := range e.vars {
			vars[k] = v
		}
		vars["timestamp"] = time.Now().Format("2006-01-02_15-04-05")
		vars["file"] = filepath.Base(file)

		disp := NewDispatcher(vars)
		for _, cmd := range e.guard.Commands {
			if err := disp.Execute(cmd); err != nil {
				log.Printf("[%s] error executing %s: %v", e.guard.Name, cmd.Verb, err)
			}
		}

		drainEvents(e.watcher.Events())

		e.scheduleCleanup(vars)
		e.scheduleFreeCache(vars)
	})

	go e.watcher.Start(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-e.watcher.Errors():
				log.Printf("[%s] watcher error: %v", e.guard.Name, err)
			case event := <-e.watcher.Events():
				e.debouncer.Trigger(event)
			}
		}
	}()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cacheTimer != nil {
		e.cacheTimer.Stop()
	}
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
	e.debouncer.Stop()
}

func (e *Engine) Watch() string {
	return e.vars["watch"]
}

func (e *Engine) scheduleCleanup(vars map[string]string) {
	if vars["trusted"] != "true" {
		return
	}

	dest := vars["dest"]
	if dest == "" {
		return
	}

	keepBackups := 0
	if kb := vars["keep_backups"]; kb != "" {
		fmt.Sscanf(kb, "%d", &keepBackups)
	}

	deleteOlderThan := ""
	if dot := vars["delete_older_than"]; dot != "" {
		if _, err := time.ParseDuration(dot); err == nil {
			deleteOlderThan = dot
		}
	}

	if keepBackups == 0 && deleteOlderThan == "" {
		return
	}

	go cleanOldBackups(dest, keepBackups, deleteOlderThan)
}

func (e *Engine) scheduleFreeCache(vars map[string]string) {
	freeCache := vars["free_cache"]
	if freeCache == "" {
		return
	}

	dur, err := time.ParseDuration(freeCache)
	if err != nil {
		log.Printf("[free_cache] invalid duration %q: %v", freeCache, err)
		return
	}

	paths := e.outputPaths(vars)
	if len(paths) == 0 {
		return
	}

	e.mu.Lock()
	if e.cacheTimer != nil {
		e.cacheTimer.Stop()
	}
	e.cacheTimer = time.AfterFunc(dur, func() {
		for _, p := range paths {
			freeOneDriveCache(p)
		}
	})
	e.mu.Unlock()
}

func (e *Engine) outputPaths(vars map[string]string) []string {
	var paths []string
	for _, cmd := range e.guard.Commands {
		switch cmd.Verb {
		case "zip", "sync":
			if len(cmd.Args) > 0 {
				if p := resolveVars(cmd.Args[0], vars); p != "" {
					paths = append(paths, p)
				}
			}
		}
	}
	return paths
}

func freeOneDriveCache(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	args := []string{"+U", "-P"}
	if info.IsDir() {
		args = append(args, "/s", "/d", filepath.Join(path, "*"))
	} else {
		args = append(args, path)
	}

	cmd := exec.Command("attrib", args...)
	if err := cmd.Run(); err != nil {
		log.Printf("[free_cache] attrib failed for %s: %v", path, err)
	} else {
		log.Printf("[free_cache] freed OneDrive cache for %s", path)
	}
}

func cleanOldBackups(destDir string, keep int, olderThan string) {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return
	}

	type zipFile struct {
		path    string
		modTime time.Time
	}

	var zips []zipFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		zips = append(zips, zipFile{
			path:    filepath.Join(destDir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(zips) == 0 {
		return
	}

	sort.Slice(zips, func(i, j int) bool {
		return zips[i].modTime.After(zips[j].modTime)
	})

	cutoff := time.Now()
	if olderThan != "" {
		if dur, err := time.ParseDuration(olderThan); err == nil {
			cutoff = cutoff.Add(-dur)
		}
	}

	for i, z := range zips {
		shouldDelete := false
		if keep > 0 && i >= keep {
			shouldDelete = true
		}
		if olderThan != "" && z.modTime.Before(cutoff) {
			shouldDelete = true
		}
		if shouldDelete {
			if err := os.Remove(z.path); err != nil {
				log.Printf("[cleanup] cannot delete %s: %v", z.path, err)
			} else {
				log.Printf("[cleanup] deleted old backup: %s", z.path)
			}
		}
	}
}

func drainEvents(ch <-chan string) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func resolveVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

