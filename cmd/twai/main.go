package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"

	"github.com/igolaizola/twai"
	"github.com/igolaizola/webcli/pkg/webff"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/peterbourgon/ff/v3/ffyaml"
)

// Build flags
var version = ""
var commit = ""
var date = ""

func main() {
	// Create signal based context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Launch command
	cmd := newCommand()
	if err := cmd.ParseAndRun(ctx, os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func newCommand() *ffcli.Command {
	fs := flag.NewFlagSet("twai", flag.ExitOnError)

	cmds := []*ffcli.Command{
		newVersionCommand(),
		newScrapeCommand(),
		newScoreCommand(),
		newEloCommand(),
	}
	port := fs.Int("port", 0, "port number")

	return &ffcli.Command{
		ShortUsage: "twai [flags] <subcommand>",
		FlagSet:    fs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				s, err := webff.New(&webff.Config{
					App:      fs.Name(),
					Commands: cmds,
					Address:  fmt.Sprintf(":%d", *port),
				})
				if err != nil {
					return err
				}
				return s.Run(ctx)
			}
			return flag.ErrHelp
		},
		Subcommands: cmds,
	}
}

func newVersionCommand() *ffcli.Command {
	return &ffcli.Command{
		Name:       "version",
		ShortUsage: "twai version",
		ShortHelp:  "print version",
		Exec: func(ctx context.Context, args []string) error {
			v := version
			if v == "" {
				if buildInfo, ok := debug.ReadBuildInfo(); ok {
					v = buildInfo.Main.Version
				}
			}
			if v == "" {
				v = "dev"
			}
			versionFields := []string{v}
			if commit != "" {
				versionFields = append(versionFields, commit)
			}
			if date != "" {
				versionFields = append(versionFields, date)
			}
			fmt.Println(strings.Join(versionFields, " "))
			return nil
		},
	}
}

func newScrapeCommand() *ffcli.Command {
	cmd := "scrape"
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	_ = fs.String("config", "", "config file (optional)")

	var cfg twai.ScrapeConfig
	fs.BoolVar(&cfg.Debug, "debug", false, "debug mode")
	fs.StringVar(&cfg.Page, "page", "home", "page to fetch (home or username)")
	fs.IntVar(&cfg.N, "n", 50, "number of tweets to fetch")
	fs.BoolVar(&cfg.Followers, "followers", false, "fetch followers stats")
	fs.StringVar(&cfg.Output, "output", "", "output file")
	fs.BoolVar(&cfg.ShowBrowser, "show-browser", false, "show browser")
	fs.StringVar(&cfg.CookieFile, "cookie-file", "cookie.txt", "cookie file")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ffyaml.Parser),
			ff.WithEnvVarPrefix("twai"),
		},
		ShortHelp: fmt.Sprintf("twai %s command", cmd),
		FlagSet:   fs,
		Exec: func(ctx context.Context, args []string) error {
			return twai.Scrape(ctx, &cfg)
		},
	}
}

func newScoreCommand() *ffcli.Command {
	cmd := "score"
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	_ = fs.String("config", "", "config file (optional)")

	var cfg twai.ScoreConfig
	fs.BoolVar(&cfg.Debug, "debug", false, "debug mode")
	fs.IntVar(&cfg.Concurrency, "concurrency", 1, "number of concurrent requests")
	fs.StringVar(&cfg.Input, "input", "", "input file (generated by scrape command)")
	fs.StringVar(&cfg.Output, "output", "", "output file (csv)")
	fs.StringVar(&cfg.Prompt, "prompt", "Rate the following tweet from 1 to 10 based on relevance, clarity, engagement, and impact. Only answer with a number.", "prompt")
	fs.StringVar(&cfg.Model, "model", "llama3", "ai model (llama3, gpt-3.5-turbo, etc)")
	fs.StringVar(&cfg.Host, "host", "http://localhost:11434/v1", "ai endpoint host (not needed for openai)")
	fs.StringVar(&cfg.Token, "token", "", "authorization token (required for openai)")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ffyaml.Parser),
			ff.WithEnvVarPrefix("twai"),
		},
		ShortHelp: fmt.Sprintf("twai %s command", cmd),
		FlagSet:   fs,
		Exec: func(ctx context.Context, args []string) error {
			return twai.Score(ctx, &cfg)
		},
	}
}

func newEloCommand() *ffcli.Command {
	cmd := "elo"
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	_ = fs.String("config", "", "config file (optional)")

	var cfg twai.EloConfig
	fs.BoolVar(&cfg.Debug, "debug", false, "debug mode")
	fs.IntVar(&cfg.Concurrency, "concurrency", 1, "number of concurrent requests")
	fs.StringVar(&cfg.Input, "input", "", "input file (generated by scrape command)")
	fs.StringVar(&cfg.Output, "output", "", "output file (csv)")
	fs.IntVar(&cfg.Iterations, "iterations", 10, "number of iterations")
	fs.StringVar(&cfg.Prompt, "prompt", "Which tweet is best based on relevance, clarity, engagement, and impact.? 1 or 2? Answer only with the number 1 or 2.", "prompt")
	fs.StringVar(&cfg.Model, "model", "llama3", "ai model (llama3, gpt-3.5-turbo, etc)")
	fs.StringVar(&cfg.Host, "host", "http://localhost:11434/v1", "ai endpoint host (not needed for openai)")
	fs.StringVar(&cfg.Token, "token", "", "authorization token (required for openai)")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ffyaml.Parser),
			ff.WithEnvVarPrefix("twai"),
		},
		ShortHelp: fmt.Sprintf("twai %s command", cmd),
		FlagSet:   fs,
		Exec: func(ctx context.Context, args []string) error {
			return twai.Elo(ctx, &cfg)
		},
	}
}
