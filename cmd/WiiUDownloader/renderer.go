package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	RENDERER_ENV_VAR          = "GSK_RENDERER"
	RENDERER_CAIRO            = "cairo"
	RENDERER_STATE_ATTEMPTING = "attempting"
	RENDERER_STATE_HEALTHY    = "ok"
	RENDERER_STATE_CAIRO      = "cairo"

	RENDERER_STATE_FILE = "renderer-state"

	GLIB_LOG_DOMAIN_ATTR = "glib_domain"
)

var gskFailureMessages = []string{
	"Failed to load shader program",
}

func isGLRenderFailure(domain, message string) bool {
	if !strings.EqualFold(strings.TrimSpace(domain), "Gsk") {
		return false
	}
	for _, failure := range gskFailureMessages {
		if strings.Contains(message, failure) {
			return true
		}
	}
	return false
}

type rendererPlan struct {
	setEnv  string
	persist string
	watch   bool
}

func planRendererLaunch(userValue, storedState string) rendererPlan {
	userValue = strings.TrimSpace(userValue)
	if userValue != "" {
		return rendererPlan{}
	}

	switch strings.TrimSpace(storedState) {
	case RENDERER_STATE_CAIRO:
		return rendererPlan{setEnv: RENDERER_CAIRO}
	case RENDERER_STATE_ATTEMPTING:
		return rendererPlan{setEnv: RENDERER_CAIRO, persist: RENDERER_STATE_CAIRO}
	default:
		return rendererPlan{persist: RENDERER_STATE_ATTEMPTING, watch: true}
	}
}

func rendererStatePathFor(userConfigDir string) string {
	return filepath.Join(configDirPath(userConfigDir), RENDERER_STATE_FILE)
}

func rendererStatePath() (string, error) {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return rendererStatePathFor(userConfigDir), nil
}

func readRendererStateFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeRendererStateFile(path, state string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), CONFIG_DIR_PERM); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(state), CONFIG_FILE_PERM)
}

func readRendererState() string {
	path, err := rendererStatePath()
	if err != nil {
		return ""
	}
	return readRendererStateFile(path)
}

func writeRendererState(state string) {
	path, err := rendererStatePath()
	if err != nil {
		return
	}
	_ = writeRendererStateFile(path, state)
}

var (
	rendererWatchingGL sync.Once
	rendererHealthy    sync.Once
)

func installGLFailureFallback(onFailure func()) {
	previous := slog.Default()
	previousWriter, previousFlags, previousPrefix := log.Writer(), log.Flags(), log.Prefix()
	slog.SetDefault(slog.New(&glFailureWatcher{
		next:      previous.Handler(),
		onFailure: onFailure,
		state:     &glFailureWatchState{},
	}))
	log.SetOutput(previousWriter)
	log.SetFlags(previousFlags)
	log.SetPrefix(previousPrefix)
}

type glFailureWatchState struct {
	mu    sync.Mutex
	fired bool
}

type glFailureWatcher struct {
	next      slog.Handler
	onFailure func()
	state     *glFailureWatchState
}

func (w *glFailureWatcher) Enabled(ctx context.Context, level slog.Level) bool {
	return w.next.Enabled(ctx, level)
}

func (w *glFailureWatcher) Handle(ctx context.Context, record slog.Record) error {
	if glRecordIsFailure(record) && w.state.take() && w.onFailure != nil {
		w.onFailure()
	}
	return w.next.Handle(ctx, record)
}

func (w *glFailureWatcher) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &glFailureWatcher{next: w.next.WithAttrs(attrs), onFailure: w.onFailure, state: w.state}
}

func (w *glFailureWatcher) WithGroup(name string) slog.Handler {
	return &glFailureWatcher{next: w.next.WithGroup(name), onFailure: w.onFailure, state: w.state}
}

func (s *glFailureWatchState) take() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired {
		return false
	}
	s.fired = true
	return true
}

func glRecordIsFailure(record slog.Record) bool {
	domain := ""
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == GLIB_LOG_DOMAIN_ATTR {
			domain = attr.Value.String()
			return false
		}
		return true
	})
	return isGLRenderFailure(domain, record.Message)
}

func watchFirstFrames(win *gtk.Window) {
	if win == nil {
		return
	}
	ticks := 0
	win.AddTickCallback(func(gtk.Widgetter, gdk.FrameClocker) bool {
		ticks++
		if ticks < 2 {
			return true
		}
		markRendererHealthy()
		return false
	})
}

func markRendererHealthy() {
	rendererHealthy.Do(func() {
		path, err := rendererStatePath()
		if err != nil {
			return
		}
		markRendererHealthyFile(path)
	})
}

func markRendererHealthyFile(path string) bool {
	if readRendererStateFile(path) != RENDERER_STATE_ATTEMPTING {
		return false
	}
	return writeRendererStateFile(path, RENDERER_STATE_HEALTHY) == nil
}

func relaunchWithCairo() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := newAppCommand(executable, os.Args[1:]...)
	cmd.Env = append(rendererEnvWithoutOverride(), RENDERER_ENV_VAR+"="+RENDERER_CAIRO)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}

func rendererEnvWithoutOverride() []string {
	env := os.Environ()
	out := env[:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, RENDERER_ENV_VAR+"=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func applyRendererPlan() rendererPlan {
	plan := planRendererLaunch(os.Getenv(RENDERER_ENV_VAR), readRendererState())
	if plan.setEnv != "" {
		os.Setenv(RENDERER_ENV_VAR, plan.setEnv)
	}
	if plan.persist != "" {
		writeRendererState(plan.persist)
	}
	return plan
}

func uiTestModeActive() bool {
	return os.Getenv("WIIU_UI_SMOKE") != "" ||
		os.Getenv("WIIU_UI_BENCH") != "" ||
		os.Getenv("WIIU_UI_STRESS") != ""
}

func enableRendererFallback() {
	if uiTestModeActive() {
		return
	}
	plan := applyRendererPlan()
	if !plan.watch {
		return
	}
	rendererWatchingGL.Do(func() {
		installGLFailureFallback(func() {
			uiIdleAdd(func() {
				writeRendererState(RENDERER_STATE_CAIRO)
				if err := relaunchWithCairo(); err != nil {
					return
				}
				os.Exit(0)
			})
		})
	})
}
