// Package recordcleaner contains the recording cleaner.
package recordcleaner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

var (
	timeNow                    = time.Now
	removeFile                 = os.Remove
	cleanAfterRetentionShorten = time.Second
)

type cleanStats struct {
	removedSegments int
	removedBytes    int64
	paths           int
	oldestStart     time.Time
	oldestPath      string
	newestStart     time.Time
	newestPath      string
}

func (s *cleanStats) merge(other cleanStats) {
	s.removedSegments += other.removedSegments
	s.removedBytes += other.removedBytes
	s.paths += other.paths
	if s.oldestStart.IsZero() || (!other.oldestStart.IsZero() && other.oldestStart.Before(s.oldestStart)) {
		s.oldestStart = other.oldestStart
		s.oldestPath = other.oldestPath
	}
	if other.newestStart.After(s.newestStart) {
		s.newestStart = other.newestStart
		s.newestPath = other.newestPath
	}
}

// Cleaner removes expired recording segments from disk.
type Cleaner struct {
	PathConfs map[string]*conf.Path
	Parent    logger.Writer

	ctx       context.Context
	ctxCancel func()

	chReloadConf chan map[string]*conf.Path
	done         chan struct{}
}

// Initialize initializes a Cleaner.
func (c *Cleaner) Initialize() {
	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	c.chReloadConf = make(chan map[string]*conf.Path)
	c.done = make(chan struct{})

	go c.run()
}

// Close closes the Cleaner.
func (c *Cleaner) Close() {
	c.ctxCancel()
	<-c.done
}

// Log implements logger.Writer.
func (c *Cleaner) Log(level logger.Level, format string, args ...any) {
	c.Parent.Log(level, "[record cleaner] "+format, args...)
}

// ReloadPathConfs is called by core.Core.
func (c *Cleaner) ReloadPathConfs(pathConfs map[string]*conf.Path) {
	select {
	case c.chReloadConf <- pathConfs:
	case <-c.ctx.Done():
	}
}

func (c *Cleaner) run() {
	defer close(c.done)

	c.doRun()

	nextRun := time.Now().Add(c.cleanInterval())
	timer := time.NewTimer(time.Until(nextRun))
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			c.doRun()
			nextRun = time.Now().Add(c.cleanInterval())
			timer.Reset(time.Until(nextRun))

		case cnf := <-c.chReloadConf:
			shortened := retentionShortened(c.PathConfs, cnf)
			c.PathConfs = cnf
			if shortened {
				// Config changes can arrive once per path. Coalesce them into an
				// early sweep, but never let later reloads postpone that sweep.
				candidate := time.Now().Add(cleanAfterRetentionShorten)
				if candidate.Before(nextRun) {
					nextRun = candidate
					resetTimer(timer, time.Until(nextRun))
				}
			}

		case <-c.ctx.Done():
			return
		}
	}
}

func resetTimer(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if delay < 0 {
		delay = 0
	}
	timer.Reset(delay)
}

// retentionShortened reports whether a reload made any path newly eligible
// for earlier deletion. A new path is included: it can point at recordings
// which predate the in-memory configuration.
func retentionShortened(oldConfs, newConfs map[string]*conf.Path) bool {
	for name, newConf := range newConfs {
		if newConf.RecordDeleteAfter == 0 {
			continue
		}
		oldConf, ok := oldConfs[name]
		if !ok || oldConf.RecordDeleteAfter == 0 ||
			newConf.RecordDeleteAfter < oldConf.RecordDeleteAfter {
			return true
		}
	}
	return false
}

func (c *Cleaner) cleanInterval() time.Duration {
	interval := 30 * 60 * time.Second

	for _, e := range c.PathConfs {
		if e.RecordDeleteAfter != 0 &&
			interval > (time.Duration(e.RecordDeleteAfter)/2) {
			interval = time.Duration(e.RecordDeleteAfter) / 2
		}
	}

	return interval
}

func (c *Cleaner) doRun() {
	now := timeNow()

	pathNames := recordstore.FindAllPathsWithSegments(c.PathConfs)
	var total cleanStats

	for _, pathName := range pathNames {
		stats, err := c.processPath(now, pathName)
		total.merge(stats)
		if err != nil {
			c.Log(logger.Warn, "cleanup failed for path %q: %v", pathName, err)
		}
	}

	if total.removedSegments > 0 {
		c.Log(logger.Info,
			"removed %d segment(s), reclaimed %d byte(s) across %d path(s), oldest=%q (%s), newest=%q (%s)",
			total.removedSegments,
			total.removedBytes,
			total.paths,
			total.oldestPath,
			total.oldestStart.Format(time.RFC3339Nano),
			total.newestPath,
			total.newestStart.Format(time.RFC3339Nano))
	}
}

func (c *Cleaner) processPath(now time.Time, pathName string) (cleanStats, error) {
	pathConf, _, err := conf.FindPathConf(c.PathConfs, pathName)
	if err != nil {
		return cleanStats{}, err
	}

	if pathConf.RecordDeleteAfter == 0 {
		return cleanStats{}, nil
	}

	stats, err := c.deleteExpiredSegments(now, pathName, pathConf)
	if err != nil {
		return stats, err
	}

	c.deleteEmptyDirs(pathConf)

	return stats, nil
}

func (c *Cleaner) deleteExpiredSegments(
	now time.Time,
	pathName string,
	pathConf *conf.Path,
) (cleanStats, error) {
	end := now.Add(-time.Duration(pathConf.RecordDeleteAfter))
	segments, err := recordstore.FindSegments(pathConf, pathName, nil, &end)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			return cleanStats{}, nil
		}
		return cleanStats{}, err
	}

	var stats cleanStats
	var removeErrors []error
	for _, seg := range segments {
		info, statErr := os.Stat(seg.Fpath)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				removeErrors = append(removeErrors, fmt.Errorf("stat %q: %w", seg.Fpath, statErr))
			}
			continue
		}
		if removeErr := removeFile(seg.Fpath); removeErr != nil {
			if !errors.Is(removeErr, os.ErrNotExist) {
				removeErrors = append(removeErrors, fmt.Errorf("remove %q: %w", seg.Fpath, removeErr))
			}
			continue
		}
		stats.removedSegments++
		stats.removedBytes += info.Size()
		if stats.oldestStart.IsZero() || seg.Start.Before(stats.oldestStart) {
			stats.oldestStart = seg.Start
			stats.oldestPath = seg.Fpath
		}
		if seg.Start.After(stats.newestStart) {
			stats.newestStart = seg.Start
			stats.newestPath = seg.Fpath
		}
	}
	if stats.removedSegments > 0 {
		stats.paths = 1
	}

	return stats, errors.Join(removeErrors...)
}

func (c *Cleaner) deleteEmptyDirs(pathConf *conf.Path) {
	recordPath := strings.ReplaceAll(pathConf.RecordPath, "%path", pathConf.Name)
	commonPath := recordstore.CommonPath(recordPath)

	filepath.WalkDir(commonPath, func(fpath string, info fs.DirEntry, err error) error { //nolint:errcheck
		if err != nil {
			return err
		}

		if info.IsDir() {
			os.Remove(fpath)
		}

		return nil
	})
}
