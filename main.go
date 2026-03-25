package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alexflint/go-arg"
	"github.com/charmbracelet/log"

	"github.com/mirovarga/filetap/db"
	"github.com/mirovarga/filetap/server"
	"github.com/mirovarga/filetap/source"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const (
	minPort = 1
	maxPort = 65535
)

type args struct {
	Local   *localCmd  `arg:"subcommand:local" help:"scan a local directory"`
	GitHub  *githubCmd `arg:"subcommand:github" help:"scan a GitHub repository"`
	Port    int        `arg:"-p,--port" default:"3000" help:"server port"`
	Cors    string     `arg:"--cors" default:"" placeholder:"ORIGINS" help:"CORS allowed origins (comma-separated)"`
	Verbose bool       `arg:"-v,--verbose" default:"false" help:"enable debug logging"`
}

func (args) Description() string {
	return "REST API for your files"
}

func (args) Version() string {
	return fmt.Sprintf("version %s (commit %s, built %s)", version, commit, date)
}

func (args) Epilogue() string {
	return `Examples:
  filetap local                            # Scan current directory
  filetap local ./docs -d 0                # Scan ./docs recursively (unlimited depth)
  filetap github owner/repo -p 8080        # Scan GitHub repo, serve on port 8080
  filetap github owner/repo --ref v1.0     # Scan a specific tag
  filetap github https://ghe.co/org/repo   # Scan GitHub Enterprise repo

API (once running):
  curl localhost:3000/api/files                                  # List files
  curl 'localhost:3000/api/files?ext[eq]=md'                     # Filter by extension
  curl 'localhost:3000/api/files?select=name,size&order=-size'   # Select fields, sort desc
  curl 'localhost:3000/api/files?name[match]=README'             # Substring match
  curl localhost:3000/api/files/{hash}/raw                       # Get raw file content`
}

type command interface {
	Run(ctx context.Context) (commandResult, error)
}

type commandResult struct {
	Source source.Source
	Depth  int
}

func validateDepth(depth int) error {
	if depth < 0 {
		return fmt.Errorf("invalid depth: %d (must be >= 0)", depth)
	}
	return nil
}

type localCmd struct {
	Dir   string `arg:"positional" default:"." help:"directory to scan"`
	Depth int    `arg:"-d,--depth" default:"1" help:"recursion depth (0 = unlimited)"`
}

func (cmd *localCmd) Run(_ context.Context) (commandResult, error) {
	if err := validateDepth(cmd.Depth); err != nil {
		return commandResult{}, err
	}

	src, err := source.NewLocal(cmd.Dir)
	if err != nil {
		return commandResult{}, err
	}

	return commandResult{Source: src, Depth: cmd.Depth}, nil
}

type githubCmd struct {
	Target string `arg:"positional,required" help:"owner/repo or full URL (e.g., https://ghe.company.com/org/repo)"`
	Ref    string `arg:"--ref" help:"branch, tag, or commit (default: repo default branch)"`
	Depth  int    `arg:"-d,--depth" default:"0" help:"recursion depth (0 = unlimited)"`
	Token  string `arg:"--token,env:GITHUB_TOKEN" help:"GitHub personal access token"`
}

func (cmd *githubCmd) Run(_ context.Context) (commandResult, error) {
	if err := validateDepth(cmd.Depth); err != nil {
		return commandResult{}, err
	}
	repo, enterpriseURL, err := parseGitHubTarget(cmd.Target)
	if err != nil {
		return commandResult{}, err
	}
	src, err := source.NewGitHub(repo, cmd.Ref, cmd.Token, enterpriseURL)
	if err != nil {
		return commandResult{}, err
	}
	return commandResult{Source: src, Depth: cmd.Depth}, nil
}

func parseGitHubTarget(target string) (repo, enterpriseURL string, err error) {
	if !strings.Contains(target, "://") {
		return target, "", nil
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub URL: %w", err)
	}

	trimmedPath := strings.TrimPrefix(parsed.Path, "/")
	parts := strings.SplitN(trimmedPath, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("missing owner/repo in URL: %s", target)
	}
	repo = parts[0] + "/" + parts[1]

	host := strings.ToLower(parsed.Hostname())
	if host != "github.com" && host != "www.github.com" {
		enterpriseURL = parsed.Scheme + "://" + parsed.Host
	}

	return repo, enterpriseURL, nil
}

func main() {
	var cliArgs args
	parser := arg.MustParse(&cliArgs)

	if parser.Subcommand() == nil {
		parser.Fail("missing subcommand")
	}

	if err := run(cliArgs); err != nil {
		log.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(cliArgs args) error {
	if cliArgs.Port < minPort || cliArgs.Port > maxPort {
		return fmt.Errorf("invalid port: %d (must be between %d and %d)", cliArgs.Port, minPort, maxPort)
	}

	sysLogger, restLogger := configureLoggers(cliArgs.Verbose)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var selectedCommand command
	switch {
	case cliArgs.Local != nil:
		selectedCommand = cliArgs.Local
	case cliArgs.GitHub != nil:
		selectedCommand = cliArgs.GitHub
	default:
		return fmt.Errorf("a subcommand is required: local or github")
	}

	result, err := selectedCommand.Run(ctx)
	if err != nil {
		return err
	}

	database, err := indexFiles(ctx, sysLogger, result.Source, result.Depth)
	if err != nil {
		return err
	}
	defer database.Close()

	var corsOrigins []string
	if cliArgs.Cors != "" {
		for _, origin := range strings.Split(cliArgs.Cors, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				corsOrigins = append(corsOrigins, origin)
			}
		}
	}

	return startServer(ctx, sysLogger, restLogger, cliArgs.Port, corsOrigins, database, result.Source)
}

func configureLoggers(verbose bool) (sys *log.Logger, rest *log.Logger) {
	baseLogger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
	if verbose {
		baseLogger.SetLevel(log.DebugLevel)
	}
	return baseLogger.WithPrefix("SYS"), baseLogger.WithPrefix("REST")
}

func indexFiles(ctx context.Context, logger *log.Logger, source source.Source, depth int) (*db.DB, error) {
	scannedFiles, err := source.List(ctx, logger, depth)
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
	}

	database, err := db.NewInMemory()
	if err != nil {
		return nil, fmt.Errorf("indexing: %w", err)
	}

	logger.Info("indexing scanned files")
	if err := database.Insert(ctx, "default", scannedFiles); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("inserting files: %w", err)
	}

	return database, nil
}

func startServer(ctx context.Context, sysLogger *log.Logger, restLogger *log.Logger,
	port int, corsOrigins []string, database *db.DB, source source.Source) error {
	srv := server.New(port, corsOrigins, database, source, restLogger)

	baseURL := fmt.Sprintf("http://localhost:%d", port)

	sysLogger.Info("server listening", "url", baseURL)

	_, _ = fmt.Fprintf(os.Stderr, "\nExamples:\n")
	_, _ = fmt.Fprintf(os.Stderr, "  curl %s/api/files\n", baseURL)
	_, _ = fmt.Fprintf(os.Stderr, "  curl '%s/api/files?ext[eq]=md'\n", baseURL)
	_, _ = fmt.Fprintf(os.Stderr, "  curl '%s/api/files?select=name,size&order=-size'\n", baseURL)
	_, _ = fmt.Fprintf(os.Stderr, "\n")

	sysLogger.Info("Ctrl+C to stop")

	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("running server: %w", err)
	}
	return nil
}
