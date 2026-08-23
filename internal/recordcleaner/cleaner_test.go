package recordcleaner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func setCleanerNow(t *testing.T, now time.Time) {
	t.Helper()
	previous := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = previous })
}

func fixedPathConf(dir, name string, retention time.Duration, record bool) *conf.Path {
	return &conf.Path{
		Name:              name,
		Record:            record,
		RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
		RecordFormat:      conf.RecordFormatFMP4,
		RecordDeleteAfter: conf.Duration(retention),
	}
}

func TestCleaner(t *testing.T) {
	setCleanerNow(t, time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local))

	dir := t.TempDir()

	const specialChars = "_-+*?^$()[]{}|"

	err := os.Mkdir(filepath.Join(dir, specialChars+"_mypath"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, specialChars+"_mypath", "2008-05-20_22-15-25-000125.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, specialChars+"_mypath", "2009-05-20_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"~^.*$": {
				Name:              "~^.*$",
				Regexp:            regexp.MustCompile("^.*$"),
				RecordPath:        filepath.Join(dir, specialChars+"_%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, specialChars+"_mypath", "2008-05-20_22-15-25-000125.mp4"))
	require.Error(t, err)

	_, err = os.Stat(filepath.Join(dir, specialChars+"_mypath", "2009-05-20_22-15-25-000427.mp4"))
	require.NoError(t, err)
}

func TestCleanerMultipleEntriesSamePath(t *testing.T) {
	setCleanerNow(t, time.Date(2009, 5, 20, 22, 15, 25, 427000, time.Local))

	dir := t.TempDir()

	err := os.Mkdir(filepath.Join(dir, "path1"), 0o755)
	require.NoError(t, err)

	err = os.Mkdir(filepath.Join(dir, "path2"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "path2", "2009-05-19_22-15-25-000427.mp4"), []byte{1}, 0o644)
	require.NoError(t, err)

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"path1": {
				Name:              "path1",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * time.Second),
			},
			"path2": {
				Name:              "path2",
				RecordPath:        filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat:      conf.RecordFormatFMP4,
				RecordDeleteAfter: conf.Duration(10 * 24 * time.Hour),
			},
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(500 * time.Millisecond)

	_, err = os.Stat(filepath.Join(dir, "path1", "2009-05-19_22-15-25-000427.mp4"))
	require.Error(t, err)

	_, err = os.Stat(filepath.Join(dir, "path1"))
	require.Error(t, err, "testing")

	_, err = os.Stat(filepath.Join(dir, "path2", "2009-05-19_22-15-25-000427.mp4"))
	require.NoError(t, err)
}

func TestCleanerInactivePathUsesPerPathRetention(t *testing.T) {
	now := time.Date(2026, 8, 22, 21, 0, 0, 0, time.Local)
	setCleanerNow(t, now)

	dir := t.TempDir()
	pathDir := filepath.Join(dir, "camera_low")
	require.NoError(t, os.Mkdir(pathDir, 0o755))
	segment := filepath.Join(pathDir, now.Add(-2*time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	require.NoError(t, os.WriteFile(segment, []byte("inactive"), 0o644))

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"camera_low": fixedPathConf(dir, "camera_low", time.Hour, false),
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	require.Eventually(t, func() bool {
		_, statErr := os.Stat(segment)
		return errors.Is(statErr, os.ErrNotExist)
	}, time.Second, 10*time.Millisecond)
}

func TestCleanerShorterRetentionTriggersCoalescedSweep(t *testing.T) {
	now := time.Date(2026, 8, 22, 21, 0, 0, 0, time.Local)
	setCleanerNow(t, now)
	previousDelay := cleanAfterRetentionShorten
	cleanAfterRetentionShorten = 20 * time.Millisecond
	t.Cleanup(func() { cleanAfterRetentionShorten = previousDelay })

	dir := t.TempDir()
	pathDir := filepath.Join(dir, "camera_high")
	require.NoError(t, os.Mkdir(pathDir, 0o755))
	segment := filepath.Join(pathDir, now.Add(-2*time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	require.NoError(t, os.WriteFile(segment, []byte("shortened"), 0o644))

	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"camera_high": fixedPathConf(dir, "camera_high", 10*time.Hour, true),
		},
		Parent: test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	time.Sleep(30 * time.Millisecond)
	_, err := os.Stat(segment)
	require.NoError(t, err)

	c.ReloadPathConfs(map[string]*conf.Path{
		"camera_high": fixedPathConf(dir, "camera_high", time.Hour, true),
	})

	require.Eventually(t, func() bool {
		_, statErr := os.Stat(segment)
		return errors.Is(statErr, os.ErrNotExist)
	}, time.Second, 10*time.Millisecond)
}

func TestCleanerReloadDoesNotPostponePeriodicSweep(t *testing.T) {
	now := time.Date(2026, 8, 22, 21, 0, 0, 0, time.Local)
	setCleanerNow(t, now)

	dir := t.TempDir()
	pathConf := fixedPathConf(dir, "camera_high", 200*time.Millisecond, true)
	c := &Cleaner{
		PathConfs: map[string]*conf.Path{"camera_high": pathConf},
		Parent:    test.NilLogger,
	}
	c.Initialize()
	defer c.Close()

	// Let the startup sweep finish before introducing the segment.
	time.Sleep(20 * time.Millisecond)
	pathDir := filepath.Join(dir, "camera_high")
	require.NoError(t, os.Mkdir(pathDir, 0o755))
	segment := filepath.Join(pathDir, now.Add(-time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	require.NoError(t, os.WriteFile(segment, []byte("periodic"), 0o644))

	stopReloads := make(chan struct{})
	doneReloads := make(chan struct{})
	go func() {
		defer close(doneReloads)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.ReloadPathConfs(map[string]*conf.Path{"camera_high": pathConf})
			case <-stopReloads:
				return
			}
		}
	}()
	defer func() {
		close(stopReloads)
		<-doneReloads
	}()

	require.Eventually(t, func() bool {
		_, statErr := os.Stat(segment)
		return errors.Is(statErr, os.ErrNotExist)
	}, time.Second, 10*time.Millisecond)
}

func TestCleanerAuditsRemovalAndReportsErrors(t *testing.T) {
	now := time.Date(2026, 8, 22, 21, 0, 0, 0, time.Local)
	setCleanerNow(t, now)

	dir := t.TempDir()
	pathDir := filepath.Join(dir, "camera_high")
	require.NoError(t, os.Mkdir(pathDir, 0o755))
	first := filepath.Join(pathDir, now.Add(-3*time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	second := filepath.Join(pathDir, now.Add(-2*time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	require.NoError(t, os.WriteFile(first, []byte("123"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("12345"), 0o644))

	logs := make(chan string, 10)
	c := &Cleaner{
		PathConfs: map[string]*conf.Path{
			"camera_high": fixedPathConf(dir, "camera_high", time.Hour, true),
		},
		Parent: test.Logger(func(level logger.Level, format string, args ...any) {
			logs <- fmt.Sprintf("%d %s", level, fmt.Sprintf(format, args...))
		}),
	}
	c.Initialize()
	defer c.Close()

	var audit string
	require.Eventually(t, func() bool {
		select {
		case audit = <-logs:
			return strings.Contains(audit, "removed 2 segment(s)")
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Contains(t, audit, "reclaimed 8 byte(s)")
	require.Contains(t, audit, "across 1 path(s)")
	require.Contains(t, audit, first)
	require.Contains(t, audit, second)

	previousRemove := removeFile
	removeFile = func(string) error { return errors.New("forced remove failure") }
	t.Cleanup(func() { removeFile = previousRemove })

	failed := filepath.Join(pathDir, now.Add(-4*time.Hour).Format("2006-01-02_15-04-05-000000")+".mp4")
	require.NoError(t, os.MkdirAll(pathDir, 0o755))
	require.NoError(t, os.WriteFile(failed, []byte("failure"), 0o644))
	c.doRun()

	var warning string
	require.Eventually(t, func() bool {
		select {
		case warning = <-logs:
			return strings.Contains(warning, "forced remove failure")
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Contains(t, warning, "cleanup failed for path")
	_, err := os.Stat(failed)
	require.NoError(t, err)
}
