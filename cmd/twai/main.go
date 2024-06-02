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
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
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

	return &ffcli.Command{
		ShortUsage: "twai [flags] <subcommand>",
		FlagSet:    fs,
		Exec: func(context.Context, []string) error {
			return flag.ErrHelp
		},
		Subcommands: []*ffcli.Command{
			newVersionCommand(),
			newScrapeCommand(),
			newScoreCommand(),
			newEloCommand(),
		},
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
	fs.StringVar(&cfg.Page, "page", "home", "page to fetch (home or username)")
	fs.IntVar(&cfg.N, "n", 50, "number of tweets to fetch")
	fs.BoolVar(&cfg.Followers, "followers", false, "fetch followers stats")
	fs.StringVar(&cfg.Output, "output", "", "output file")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ff.PlainParser),
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
	fs.StringVar(&cfg.Input, "input", "", "input file")
	fs.StringVar(&cfg.Output, "output", "", "output file")
	fs.StringVar(&cfg.Prompt, "prompt", "Rate the following tweet from 1 to 10 based on relevance, clarity, engagement, and impact. Only answer with a number.", "prompt")
	fs.StringVar(&cfg.Model, "model", "llama3", "ai model (llama3, gpt-3.5-turbo, etc)")
	fs.StringVar(&cfg.Host, "host", "http://localhost:11434/v1", "ai endpoint host (not needed for openai)")
	fs.StringVar(&cfg.Token, "token", "", "authorization token (required for openai)")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ff.PlainParser),
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
	fs.StringVar(&cfg.Input, "input", "", "input file")
	fs.StringVar(&cfg.Output, "output", "", "output file")
	fs.IntVar(&cfg.Iterations, "iterations", 10, "number of iterations")
	fs.StringVar(&cfg.Prompt, "prompt", "Rate the following tweet from 1 to 10 based on relevance, clarity, engagement, and impact. Only answer with a number.", "prompt")
	fs.StringVar(&cfg.Model, "model", "llama3", "ai model (llama3, gpt-3.5-turbo, etc)")
	fs.StringVar(&cfg.Host, "host", "http://localhost:11434/v1", "ai endpoint host (not needed for openai)")
	fs.StringVar(&cfg.Token, "token", "", "authorization token (required for openai)")

	return &ffcli.Command{
		Name:       cmd,
		ShortUsage: fmt.Sprintf("twai %s [flags] <key> <value data...>", cmd),
		Options: []ff.Option{
			ff.WithConfigFileFlag("config"),
			ff.WithConfigFileParser(ff.PlainParser),
			ff.WithEnvVarPrefix("twai"),
		},
		ShortHelp: fmt.Sprintf("twai %s command", cmd),
		FlagSet:   fs,
		Exec: func(ctx context.Context, args []string) error {
			return twai.Elo(ctx, &cfg)
		},
	}
}
