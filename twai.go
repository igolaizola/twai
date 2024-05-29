package twai

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/igolaizola/twai/pkg/openai"
	"github.com/igolaizola/twai/pkg/twitter"
)

type Tweets struct {
	Link      string `json:"link"`
	Score     int    `json:"score"`
	Views     int    `json:"views"`
	Followers int    `json:"followers"`
}

// Run runs the twai process.
func Run(ctx context.Context, page string, n int) error {
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
	posts, err := b.Posts(ctx, page, n)
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
	fmt.Println("Score\tViews\tFollowers\tLink")
	for _, tw := range tws {
		fmt.Printf("%d\t%d\t%d\t%s\n", tw.Score, tw.Views, tw.Followers, tw.Link)
	}
	return nil
}

var numberRegex = regexp.MustCompile(`\d+`)
