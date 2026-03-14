// Package satellite provides satellite runtime configuration and Pi RPC bridge functionality.
package satellite

import (
	"log/slog"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceDuration is how long to wait after the last fsnotify event before
// emitting a ContextFileEvent. Windows fires multiple events per save.
const debounceDuration = 300 * time.Millisecond

// ContextDir returns the platform-specific path for the satellite context directory.
func ContextDir() string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "daao", "context")
	case "darwin":
		home, _ := os.UserHomeDir()
		if home == "" {
			home = "/tmp"
		}
		return filepath.Join(home, "Library", "Application Support", "daao", "context")
	default: // linux
		return "/etc/daao/context"
	}
}

// StandardFiles lists the well-known context files and their starter content.
var StandardFiles = []struct {
	Name    string
	Content string
}{
	{
		Name: "systeminfo.md",
		Content: `# systeminfo.md

## Role
<!-- Describe this machine's primary role (e.g., "Primary web server", "CI runner") -->

## Services
<!-- List running services with ports, e.g.:
- **nginx** — port 80/443
- **postgres** — port 5432
-->

## Hardware
<!-- CPU, RAM, disk, GPU if relevant -->

## Network
<!-- Hostname, IP ranges, VLANs -->

## OS
<!-- OS version, kernel, package manager -->
`,
	},
	{
		Name: "runbooks.md",
		Content: `# runbooks.md

## Standard Procedures

### Restart Services
<!-- Step-by-step for common restarts -->

### Log Locations
<!-- Where to find logs for each service -->

### Backup & Restore
<!-- How to back up and restore critical data -->

### Emergency Contacts
<!-- Who to notify for different incident types -->
`,
	},
	{
		Name: "alerts.md",
		Content: `# alerts.md

## Known Alert Conditions

<!-- Format:
### Alert Name
- **Trigger:** what causes it
- **Severity:** low/medium/high/critical
- **Resolution:** step-by-step fix
-->
`,
	},
	{
		Name: "topology.md",
		Content: `# topology.md

## Upstream Dependencies
<!-- Services/hosts this machine depends on -->

## Downstream Consumers
<!-- Services/hosts that depend on this machine -->

## Network Diagram
<!-- ASCII or description of how this host fits in the broader network -->
`,
	},
	{
		Name: "secrets-policy.md",
		Content: `# secrets-policy.md

## Available Credentials
<!-- List what credentials this host has access to — NO actual values, just references.
Example:
- DB_PASSWORD: PostgreSQL password for 'app' user (stored in Vault at secret/db/app)
- API_KEY: Stripe API key (env var STRIPE_KEY, set in /etc/daao/daemon.env)
-->

## Access Policy
<!-- Who/what can access these credentials and under what conditions -->
`,
	},
	{
		Name: "history.md",
		Content: `# history.md

## Recent Changes
<!-- Log significant changes, deployments, and incidents in reverse chronological order.
Format:
### YYYY-MM-DD — Brief description
- What changed
- Why it changed
- Any issues encountered
-->
`,
	},
	{
		Name: "monitoring.md",
		Content: `# monitoring.md

## Monitoring Tools
<!-- What tools are installed (Datadog agent, Prometheus node_exporter, etc.) -->

## Key Metrics & Thresholds
<!-- Metric → alert threshold → action -->

## Dashboards
<!-- Links or descriptions of relevant dashboards -->
`,
	},
	{
		Name: "dependencies.md",
		Content: `# dependencies.md

## External Services
<!-- Third-party APIs or cloud services this host calls -->

## Internal Services
<!-- Internal microservices or databases this host depends on -->

## Blast Radius
<!-- What breaks if this host goes down -->
`,
	},
}

// ContextFileEvent is emitted by ContextWatcher when a file changes.
type ContextFileEvent struct {
	FilePath string // relative filename, e.g. "systeminfo.md"
	Content  string // full file content (empty string on delete)
	Deleted  bool
}

// ContextWatcher watches the satellite context directory and emits events
// when files are created, written, or deleted.
type ContextWatcher struct {
	dir     string
	watcher *fsnotify.Watcher
	events  chan ContextFileEvent
	done    chan struct{}
	once    sync.Once
}

// NewContextWatcher creates a watcher for the given directory.
// Call Start() to begin watching.
func NewContextWatcher(dir string) *ContextWatcher {
	return &ContextWatcher{
		dir:    dir,
		events: make(chan ContextFileEvent, 32),
		done:   make(chan struct{}),
	}
}

// Events returns the channel of file change events.
func (w *ContextWatcher) Events() <-chan ContextFileEvent {
	return w.events
}

// Start begins watching the context directory.
func (w *ContextWatcher) Start() error {
	if err := os.MkdirAll(w.dir, 0755); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(w.dir); err != nil {
		watcher.Close()
		return err
	}
	w.watcher = watcher

	go w.loop()
	return nil
}

// Stop shuts down the watcher.
func (w *ContextWatcher) Stop() {
	w.once.Do(func() {
		close(w.done)
		if w.watcher != nil {
			w.watcher.Close()
		}
	})
}

func (w *ContextWatcher) loop() {
	// pending tracks the latest event type per filename, waiting to be flushed.
	type pendingEvent struct {
		deleted bool
	}
	pending := make(map[string]pendingEvent)
	var mu sync.Mutex
	var timer *time.Timer

	flush := func() {
		mu.Lock()
		toSend := pending
		pending = make(map[string]pendingEvent)
		mu.Unlock()

		for relPath, ev := range toSend {
			if ev.deleted {
				select {
				case w.events <- ContextFileEvent{FilePath: relPath, Deleted: true}:
				default:
					slog.Info(fmt.Sprintf("ContextWatcher: event channel full, dropping delete for %s", relPath), "component", "satellite")
				}
				continue
			}
			absPath := filepath.Join(w.dir, relPath)
			content, err := os.ReadFile(absPath)
			if err != nil {
				slog.Error(fmt.Sprintf("ContextWatcher: failed to read %s: %v", relPath, err), "component", "satellite")
				continue
			}
			select {
			case w.events <- ContextFileEvent{FilePath: relPath, Content: string(content)}:
			default:
				slog.Info(fmt.Sprintf("ContextWatcher: event channel full, dropping write for %s", relPath), "component", "satellite")
			}
		}
	}

	for {
		select {
		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if !isMDFile(event.Name) {
				continue
			}
			relPath := filepath.Base(event.Name)
			deleted := event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename)

			mu.Lock()
			pending[relPath] = pendingEvent{deleted: deleted}
			mu.Unlock()

			// Reset debounce timer on every event.
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceDuration, flush)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Error(fmt.Sprintf("ContextWatcher: fsnotify error: %v", err), "component", "satellite")
		}
	}
}

// SeedStandardFiles writes any missing standard context files to the directory.
func SeedStandardFiles(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, f := range StandardFiles {
		path := filepath.Join(dir, f.Name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(f.Content), 0644); err != nil {
				slog.Error(fmt.Sprintf("ContextWatcher: failed to seed %s: %v", f.Name, err), "component", "satellite")
			}
		}
	}
	return nil
}

// ReadAllContextFiles reads all .md files from the directory and returns their contents.
func ReadAllContextFiles(dir string) map[string]string {
	result := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || !isMDFile(entry.Name()) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err == nil {
			result[entry.Name()] = string(content)
		}
	}
	return result
}

func isMDFile(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".md")
}
