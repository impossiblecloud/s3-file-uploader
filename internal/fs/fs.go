package fs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/adidenko/s3-file-uploader/internal/cfg"

	"github.com/fsnotify/fsnotify"
)

// Check if fs event is the one we care about
func isValidFsEvent(event fsnotify.Event) bool {

	// fsnotify does not support CLOSE_WRITE events, so we need to watch for CREATE.
	// Which means something else needs to move files (mv SRC DST) to the directory we watch.
	if event.Op&fsnotify.Create == fsnotify.Create {
		return true
	}
	// if event.Op&fsnotify.Write == fsnotify.Write {
	// 	return true
	// }
	// if event.Op&fsnotify.Remove == fsnotify.Remove {
	// 	return true
	// }

	return false
}

func fsWatch(ctx context.Context, comm *chan cfg.Message, watcher *fsnotify.Watcher, config cfg.AppConfig) {
	for {
		select {
		case <-ctx.Done():
			config.Applog.Info("fsWatch function exiting")
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if isValidFsEvent(event) {
				config.Applog.Infof("Detected file: %q (%v)", event.Name, event.Op)
				if len(*comm) < config.WorkersCannelSize {
					*comm <- cfg.Message{File: event.Name}
				} else {
					config.Metrics.ChannelFullEvents.WithLabelValues().Inc()
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			config.Applog.Error(err)
		}
	}
}

func fsScan(comm *chan cfg.Message, config cfg.AppConfig) {
	entries, err := os.ReadDir(config.PathToWatch)
	if err != nil {
		config.Applog.Fatal(err)
	}

	for _, e := range entries {
		//config.Applog.Infof("Found file %q", e.Name())
		*comm <- cfg.Message{File: filepath.Join(config.PathToWatch, e.Name())}
	}
}

// ScanDirectory periodically scans the directory and sends files to process into the channel for workers
func ScanDirectory(ctx context.Context, comm *chan cfg.Message, config cfg.AppConfig) {
	tick := time.NewTicker(config.ScanInterval)

	config.Applog.Info("Directory scanner started")
	// Keep fireing until we receive exit signal
	for {
		select {
		// Exit signal
		case <-ctx.Done():
			config.Applog.Info("Directory scanner exiting")
			return
		// Tick event
		case <-tick.C:
			//config.Applog.Info("Tick event")
			fsScan(comm, config)
		}
	}

}

// WatchDirectory uses fsnotify to watch directory for events
func WatchDirectory(ctx context.Context, comm *chan cfg.Message, config cfg.AppConfig) {
	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		config.Applog.Fatal(err)
	}
	defer watcher.Close()

	// Start listening for events.
	go fsWatch(ctx, comm, watcher, config)

	// Add a path.
	err = watcher.Add(config.PathToWatch)
	if err != nil {
		config.Applog.Fatalf("Failed to watch %q path: %s", config.PathToWatch, err.Error())
	}

	// Watcher setup done, exiting
	config.Applog.Infof("Started fsnotify watcher for %q path", config.PathToWatch)
	<-ctx.Done()
	config.Applog.Info("WatchDirectory function exiting")
}

// EncryptFile encrypts a file with gpg tool
func EncryptFile(config cfg.AppConfig, filename string) error {
	if !config.Encrypt {
		return nil
	}

	// Original command: gpg -c --verbose --batch --yes --passphrase $GPG_PASSWORD -o /data/enc/$f /data/sql/$f
	file := filepath.Base(filename)
	srcFile := filename
	if config.Gzip {
		srcFile = filepath.Join(config.GzipDir, file+".tgz")
	}
	encFile := filepath.Join(config.EncryptDir, file+".tgz")

	// Use external gpg tool to make sure we can decrypt easily using the same tool
	cmd := exec.Command("gpg", "-c", "--batch", "--yes", "--passphrase", config.GpgPassword, "-o", encFile, srcFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		// "gpg -c" always returns exit code 2, so we need to work that around by checking size of encrypted file
		if fi, err := os.Stat(encFile); err == nil {
			if fi.Size() > 0 {
				// Encrypted file is not empty, we can exit
				return nil
			}
		}
		return fmt.Errorf("error executing gpg CLI command for %q: %s: %s", filename, err.Error(), string(output))
	}
	return nil
}

// GzipFile gzips a file
func GzipFile(config cfg.AppConfig, filename string) error {
	if !config.Gzip {
		return nil
	}

	file := filepath.Base(filename)
	gzipFile := filepath.Join(config.GzipDir, file+".tgz")

	// Use external tar+gzip tool to make sure we can unpack easily
	cmd := exec.Command("tar", "czf", gzipFile, "-C", config.PathToWatch, file)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error executing tgz CLI command for %q: %s: %s", filename, err.Error(), string(output))
	}
	return nil
}

// DeleteFile deletes a file and all temporary ones (gzip and encrypted)
func DeleteFile(config cfg.AppConfig, filename string) error {
	file := filepath.Base(filename)
	gzipFile := filepath.Join(config.GzipDir, file+".tgz")
	encFile := filepath.Join(config.EncryptDir, file+".tgz")

	if err := os.Remove(filename); err != nil {
		if err := os.Remove(filename); err != nil {
			config.Applog.Error(err)
			return err
		}
	}

	if config.Gzip {
		if err := os.Remove(gzipFile); err != nil {
			config.Applog.Error(err)
			return err
		}
	}

	if config.Encrypt {
		if err := os.Remove(encFile); err != nil {
			config.Applog.Error(err)
			return err
		}
	}

	return nil
}
