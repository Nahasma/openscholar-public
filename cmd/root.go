package cmd

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openscholar/openscholar/internal/app"
	"github.com/openscholar/openscholar/internal/command"
	"github.com/openscholar/openscholar/internal/command/builtin"
	"github.com/openscholar/openscholar/internal/command/custom"
	"github.com/openscholar/openscholar/internal/config"
	"github.com/openscholar/openscholar/internal/db"
	"github.com/openscholar/openscholar/internal/debug"
	initwizard "github.com/openscholar/openscholar/internal/init"
	"github.com/openscholar/openscholar/internal/llm/models"
	"github.com/openscholar/openscholar/internal/llm/tools"
	"github.com/openscholar/openscholar/internal/pubsub"
	"github.com/openscholar/openscholar/internal/session"
	"github.com/openscholar/openscholar/internal/skillbank"
	"github.com/openscholar/openscholar/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "openscholar",
	Short: "AI-powered academic paper writing assistant",
	Long: `OpenScholar is a terminal-based AI assistant for academic paper writing.
It provides an interactive chat interface to help researchers write LaTeX papers,
manage references, and structure academic documents.`,
	Example: `
  # Interactive mode
  openscholar

  # Non-interactive mode
  openscholar -p "Generate the softmax formula in LaTeX"

  # Initialize a new paper project
  openscholar init --template neurips2026

  # Specify a model
  openscholar --model sonnet -p "hello"
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flag("help").Changed {
			return cmd.Help()
		}

		// Ensure TERM is set for proper color support
		if os.Getenv("TERM") == "" {
			os.Setenv("TERM", "xterm-256color")
		}

		prompt, _ := cmd.Flags().GetString("prompt")
		cwd, _ := cmd.Flags().GetString("cwd")

		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				return fmt.Errorf("failed to change directory: %v", err)
			}
		}
		if cwd == "" {
			c, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current working directory: %v", err)
			}
			cwd = c
		}

		_, err := config.Load(cwd)
		if err != nil {
			// Check if this is a "no provider configured" error (first run)
			if strings.Contains(err.Error(), "no provider configured") {
				fmt.Println("未检测到配置，启动初始化向导...")
				if wizardErr := initwizard.RunHardInit(cwd); wizardErr != nil {
					return fmt.Errorf("配置向导失败: %w", wizardErr)
				}
				config.Reset()
				if _, err = config.Load(cwd); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// --debug flag: enable session debug logging
		debugFlag, _ := cmd.Flags().GetBool("debug")
		if debugFlag {
			config.Get().Debug = true
		}

		// Create session log directory (timestamped folder under logs/)
		var sessionLogDir string
		sessionLogDir, _ = debug.CreateSessionDir(config.LogDir())

		// Redirect log/slog output to session folder (or fallback to flat logDir)
		logPath := filepath.Join(config.LogDir(), "app.log")
		if sessionLogDir != "" {
			logPath = filepath.Join(sessionLogDir, "app.log")
		}
		if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err == nil {
			logFile, fileErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if fileErr == nil {
				defer logFile.Close()
				log.SetOutput(logFile)
				logLevel := slog.LevelInfo

				// --verbose flag
				verbose, _ := cmd.Flags().GetBool("verbose")
				if verbose {
					logLevel = slog.LevelDebug
				}
				slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: logLevel})))
				slog.Info("openscholar started", "cwd", cwd)
			}
		}

		conn, err := db.Connect()
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Inject session log directory into context for agent logging
		if sessionLogDir != "" {
			ctx = debug.WithSessionLogDir(ctx, sessionLogDir)
		}

		application, err := app.New(ctx, conn)
		if err != nil {
			return err
		}
		defer application.Shutdown()

		// --model flag: override model
		modelFlag, _ := cmd.Flags().GetString("model")
		if modelFlag != "" {
			ref, err := models.ResolveModelRef(modelFlag)
			if err == nil {
				if err := application.SetModelProvider(ref.Provider, ref.ModelID); err != nil {
					return fmt.Errorf("failed to set model: %v", err)
				}
			} else if providerName, modelID, ok := splitRootProviderModel(modelFlag); ok {
				if err := application.SetModelProvider(providerName, modelID); err != nil {
					return fmt.Errorf("failed to set model: %v", err)
				}
			} else {
				return fmt.Errorf("model not found: %s", modelFlag)
			}
		}

		resumeSelector, _ := cmd.Flags().GetString("resume")
		if resumeSelector == "__PICKER__" {
			resumeSelector = ""
		}
		continueLatest, _ := cmd.Flags().GetBool("continue")

		// Non-interactive mode
		if prompt != "" {
			resumeFlagChanged := cmd.Flag("resume").Changed
			if resumeFlagChanged && strings.TrimSpace(resumeSelector) == "" && !continueLatest {
				return fmt.Errorf("--resume requires an exact session id or exact title when used with --prompt")
			}
			if continueLatest || strings.TrimSpace(resumeSelector) != "" {
				mode := session.ResolveExact
				selector := strings.TrimSpace(resumeSelector)
				if continueLatest {
					mode = session.ResolveLatest
					selector = ""
				}
				resolved, rerr := application.Resume.Resolve(ctx, session.ResumeRequest{
					Mode:        mode,
					Selector:    selector,
					ProjectPath: cwd,
					Limit:       5,
				})
				if rerr != nil {
					return rerr
				}
				if len(resolved.Ambiguous) > 1 {
					return fmt.Errorf("resume selector is ambiguous in non-interactive mode; please use an exact session id or exact title")
				}
				if !resolved.Found {
					return fmt.Errorf("no session matched resume selector: %s", selector)
				}
				return application.RunNonInteractiveInSession(ctx, resolved.Session.ID, prompt)
			}
			return application.RunNonInteractive(ctx, prompt)
		}

		// Create command system
		registry := command.NewRegistry()
		builtin.RegisterAll(registry, application)
		custom.LoadAll(registry)
		custom.LoadSkillBankCommands(registry, application.SkillBank)
		pathActivator := skillbank.NewPathsActivator(application.SkillBank)
		commandActivator := custom.NewSkillActivator(application.SkillBank, registry)
		application.CoderAgent.SetFileToolUsageNotifier(func(sessionID string, evt tools.FileToolUsageEvent) {
			// Fail-open: activation should never block tool execution.
			_, _ = pathActivator.ActivateByPaths(context.Background(), evt.Workspace, evt.Paths)
			commandActivator.OnFileToolUsage(sessionID, evt)
		})
		dispatcher := command.NewDispatcher(registry)

		// Interactive mode — TUI (default: new session; --continue or --resume[-selector])
		resumeRequested := continueLatest || cmd.Flag("resume").Changed
		var tuiOpts []tui.Option
		tuiOpts = append(tuiOpts, tui.WithResumeFlags(continueLatest, resumeSelector))
		if cwd != "" && cmd.Flag("cwd").Changed {
			tuiOpts = append(tuiOpts, tui.WithCWDExplicit())
		}
		initialScreenMode := tui.ScreenModeFromEnv()
		tuiOpts = append(tuiOpts, tui.WithScreenMode(initialScreenMode))
		mainScreenController := tui.NewMainScreenOutputController(tui.MainScreenResetModeFromEnv())
		tuiOpts = append(tuiOpts, tui.WithMainScreenRenderer(mainScreenController))

		var tuiTrace *debug.TUITrace
		if sessionLogDir != "" && config.SessionLogEnabled() {
			tuiTrace, err = debug.NewTUITrace(sessionLogDir, initialScreenMode.IsFullscreen())
			if err != nil {
				slog.Warn("failed to initialize tui trace", "error", err)
			} else {
				defer tuiTrace.Close()
				tuiOpts = append(tuiOpts, tui.WithDebugTrace(tuiTrace))
			}
		}

		var programOpts []tea.ProgramOption
		if initialScreenMode.IsFullscreen() {
			programOpts = append(programOpts,
				tea.WithAltScreen(),
				tea.WithMouseCellMotion(),
			)
		}
		if tuiTrace != nil {
			out := tuiTrace.WrapOutput(os.Stdout)
			out = mainScreenController.WrapOutput(out)
			programOpts = append(programOpts, tea.WithOutput(out))
		} else {
			programOpts = append(programOpts, tea.WithOutput(mainScreenController.WrapOutput(os.Stdout)))
		}

		program := tea.NewProgram(
			tui.New(ctx, application, resumeRequested, dispatcher, tuiOpts...),
			programOpts...,
		)

		ch, cancelSubs := setupSubscriptions(application, ctx)

		tuiCtx, tuiCancel := context.WithCancel(ctx)
		var tuiWg sync.WaitGroup
		tuiWg.Add(1)

		go func() {
			defer tuiWg.Done()
			for {
				select {
				case <-tuiCtx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						return
					}
					program.Send(msg)
				}
			}
		}()

		cleanup := func() {
			application.Shutdown()
			cancelSubs()
			tuiCancel()
			tuiWg.Wait()
		}

		_, err = program.Run()
		cleanup()

		if err != nil {
			return fmt.Errorf("TUI error: %v", err)
		}
		return nil
	},
}

func splitRootProviderModel(input string) (models.ModelProvider, string, bool) {
	idx := strings.Index(input, ":")
	if idx <= 0 {
		return "", "", false
	}
	providerName, ok := models.ResolveProvider(input[:idx])
	if !ok {
		return "", "", false
	}
	modelID := strings.TrimSpace(input[idx+1:])
	if modelID == "" {
		return "", "", false
	}
	return providerName, modelID, true
}

func setupSubscriber[T any](
	ctx context.Context,
	wg *sync.WaitGroup,
	subscriber func(context.Context) <-chan pubsub.Event[T],
	outputCh chan<- tea.Msg,
) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		subCh := subscriber(ctx)
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event, ok := <-subCh:
				if !ok {
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(5 * time.Second)
				select {
				case outputCh <- event:
				case <-timer.C:
					slog.Debug("setupSubscriber: event forwarding timeout")
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func setupSubscriptions(app *app.App, parentCtx context.Context) (chan tea.Msg, func()) {
	ch := make(chan tea.Msg, 512)
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(parentCtx)

	setupSubscriber(ctx, &wg, app.Sessions.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.Messages.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.Permissions.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.CoderAgent.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.ClarificationBroker.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.PlanApprovalBroker.Subscribe, ch)
	setupSubscriber(ctx, &wg, app.CheckpointBroker.Subscribe, ch)
	if app.ResearchEvents != nil {
		setupSubscriber(ctx, &wg, app.ResearchEvents.Subscribe, ch)
	}
	if app.MemoryEvents != nil {
		setupSubscriber(ctx, &wg, app.MemoryEvents.Subscribe, ch)
	}
	if app.TaskRegistry != nil {
		setupSubscriber(ctx, &wg, app.TaskRegistry.Subscribe, ch)
	}

	cleanupFunc := func() {
		cancel()
		waitCh := make(chan struct{})
		go func() {
			wg.Wait()
			close(waitCh)
		}()
		select {
		case <-waitCh:
			close(ch)
		case <-time.After(5 * time.Second):
			close(ch)
		}
	}
	return ch, cleanupFunc
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().BoolP("help", "h", false, "Help")
	rootCmd.Flags().StringP("cwd", "c", "", "Working directory")
	rootCmd.Flags().StringP("prompt", "p", "", "Non-interactive prompt")
	rootCmd.Flags().StringP("resume", "r", "", "Resume by session id/title/query (no value opens picker)")
	rootCmd.Flags().Lookup("resume").NoOptDefVal = "__PICKER__"
	rootCmd.Flags().BoolP("new", "n", false, "Start a new session (deprecated, now default)")
	rootCmd.Flags().String("model", "", "Model name or ID (e.g. sonnet, haiku, gpt-4.1)")
	rootCmd.Flags().Bool("verbose", false, "Enable debug logging")
	rootCmd.Flags().Bool("continue", false, "Resume latest session from current project")
	rootCmd.Flags().Bool("debug", false, "Enable session debug logging to .openscholar/logs/")
	rootCmd.Flags().String("output-format", "text", "Output format for non-interactive mode (text or json)")
	rootCmd.Flags().Int("max-turns", 0, "Maximum agent turns for non-interactive mode")
}
