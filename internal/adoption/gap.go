package adoption

import (
	"errors"
	"os"
	"path/filepath"
)

// MarkGap is independent of SQLite so a lost observation under a writer lock
// cannot later be scored as a negative. It is sticky for this collector directory:
// reset means starting a fresh experiment, never deleting a failed observation.
func MarkGap(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("collector gap directory invalid")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("collector gap directory unavailable")
	}
	f, err := os.OpenFile(filepath.Join(dir, "coverage-gap"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("collector gap marker unavailable")
	}
	_, err = f.WriteString("coverage gap observed; rates are unavailable for this collector\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// GapSeen treats an unreadable marker as a gap, never a clean collection.
func GapSeen(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, "coverage-gap"))
	return !os.IsNotExist(err)
}
