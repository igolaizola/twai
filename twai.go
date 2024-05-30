package twai

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/igolaizola/twai/pkg/openai"
	"github.com/igolaizola/twai/pkg/twitter"
)

type Tweets struct {
	Score     int    `json:"score" csv:"score"`
	Views     int    `json:"views" csv:"views"`
	Followers int    `json:"followers" csv:"followers"`
	Link      string `json:"link" csv:"link"`
}

// Run runs the twai process.
func Run(ctx context.Context, page string, n int, followers bool, output string) error {
	log.Println("running")
	defer log.Println("finished")

	c := openai.New(&openai.Config{
		Model: "llama3",
		Host:  "http://localhost:11434/v1",
	})

	b := twitter.NewBrowser(&twitter.BrowserConfig{
		Wait:        1 * time.Second,
		CookieStore: twitter.NewCookieStore("cookie.txt"),
	})
	if err := b.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = b.Stop() }()

	// Get tweets
	posts, err := b.Posts(ctx, page, n, followers)
	if err != nil {
		return err
	}

	// Ask for relevance of each tweet
	var tws []Tweets
	for _, post := range posts {
		resp, err := c.ChatCompletion(ctx, "From 1 to 10, how relevant is this tweet for a software engineer audience? Answer only with a number.\nTweet:\n"+post.Text)
		if err != nil {
			return err
		}
		match := numberRegex.FindString(resp)
		var n int
		if match == "" {
			log.Println("no number found in response")
		} else {
			candidate, err := strconv.Atoi(match)
			if err != nil {
				log.Println("error parsing number from response")
			}
			n = candidate
		}
		tws = append(tws, Tweets{
			Link:      fmt.Sprintf("https://x.com/%s/status/%s", post.UserID, post.ID),
			Score:     n,
			Views:     post.Views,
			Followers: post.UserFollowers,
		})
	}

	// Order tweets by score and views
	sort.Slice(tws, func(i, j int) bool {
		return tws[i].Score > tws[j].Score || (tws[i].Score == tws[j].Score && tws[i].Views > tws[j].Views)
	})

	// Marshal tweets to CSV
	data, err := gocsv.MarshalBytes(&tws)
	if err != nil {
		return fmt.Errorf("couldn't marshal tweets to csv: %w", err)
	}
	// Write to file if output is provided
	if output != "" {
		if err := os.WriteFile(output, data, 0644); err != nil {
			return fmt.Errorf("couldn't write tweets to file: %w", err)
		}
	} else {
		fmt.Println(string(data))
	}
	return nil
}

var numberRegex = regexp.MustCompile(`\d+`)
