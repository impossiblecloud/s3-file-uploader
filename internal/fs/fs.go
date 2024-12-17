package fs

import (
	"context"

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
				if config.ExitOnFilename != "" && event.Name == config.ExitOnFilename {
					config.Applog.Infof("Exiting on file: %q", event.Name)
					config.CancelFunction()
					return
				}
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
