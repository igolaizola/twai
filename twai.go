package twai

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/igolaizola/twai/pkg/openai"
	"github.com/igolaizola/twai/pkg/twitter"
)

type ScrapeConfig struct {
	Debug       bool
	CookieFile  string
	ShowBrowser bool
	Page        string
	N           int
	Followers   bool
	Output      string
}

func Scrape(ctx context.Context, cfg *ScrapeConfig) error {
	log.Println("running")
	defer log.Println("finished")

	b := twitter.NewBrowser(&twitter.BrowserConfig{
		Wait:        1 * time.Second,
		CookieStore: twitter.NewCookieStore(cfg.CookieFile),
		Headless:    !cfg.ShowBrowser,
	})
	if err := b.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = b.Stop() }()

	// Get tweets
	posts, err := b.Posts(ctx, cfg.Page, cfg.N, cfg.Followers)
	if err != nil {
		return err
	}

	// Order tweets by score and views
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Views > posts[j].Views
	})

	// Marshal tweets to CSV
	data, err := gocsv.MarshalBytes(&posts)
	if err != nil {
		return fmt.Errorf("couldn't marshal tweets to csv: %w", err)
	}
	// Write to file if output is provided
	if cfg.Output != "" {
		if err := os.WriteFile(cfg.Output, data, 0644); err != nil {
			return fmt.Errorf("couldn't write tweets to file: %w", err)
		}
		fmt.Println("created file:", cfg.Output)
	} else {
		fmt.Println(string(data))
	}
	return nil
}

type Tweet struct {
	Score int `json:"score" csv:"score"`

	Comments int `json:"comments" csv:"comments"`
	Retweets int `json:"retweets" csv:"retweets"`
	Likes    int `json:"likes" csv:"likes"`
	Views    int `json:"views" csv:"views"`

	Time time.Time `json:"time" csv:"time"`
	Text string    `json:"text" csv:"text"`
	Link string    `json:"link" csv:"link"`
}

type ScoreConfig struct {
	Debug  bool
	Input  string
	Output string
	Prompt string
	Model  string
	Host   string
	Token  string
}

func Score(ctx context.Context, cfg *ScoreConfig) error {
	log.Println("running")
	defer log.Println("finished")

	var posts []*twitter.Post
	b, err := os.ReadFile(cfg.Input)
	if err != nil {
		return fmt.Errorf("couldn't read tweets from file: %w", err)
	}
	if err := gocsv.UnmarshalBytes(b, &posts); err != nil {
		return fmt.Errorf("couldn't unmarshal tweets from csv: %w", err)
	}
	if len(posts) < 1 {
		return fmt.Errorf("need at least 1 tweet to score")
	}

	c := openai.New(&openai.Config{
		Debug: cfg.Debug,
		Model: cfg.Model,
		Host:  cfg.Host,
		Token: cfg.Token,
	})

	// Ask for relevance of each tweet
	var tws []*Tweet
	for _, post := range posts {
		log.Printf("ai %d/%d\n", len(tws)+1, len(posts))
		resp, err := c.ChatCompletion(ctx, "Rate the following tweet from 1 to 10 based on relevance, clarity, engagement, and impact. Only answer with a number.\n\n"+post.Text)
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
		tws = append(tws, &Tweet{
			Score: n,

			Comments: post.Comments,
			Retweets: post.Retweets,
			Likes:    post.Likes,
			Views:    post.Views,

			Time: post.Time,
			Text: post.Text,
			Link: fmt.Sprintf("https://x.com/%s/status/%s", post.UserID, post.ID),
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
	if cfg.Output != "" {
		if err := os.WriteFile(cfg.Output, data, 0644); err != nil {
			return fmt.Errorf("couldn't write tweets to file: %w", err)
		}
		fmt.Println("created file:", cfg.Output)
	} else {
		fmt.Println(string(data))
	}
	return nil
}

type EloConfig struct {
	Debug      bool
	Input      string
	Output     string
	Iterations int
	Model      string
	Host       string
	Prompt     string
	Token      string
}

func Elo(ctx context.Context, cfg *EloConfig) error {
	log.Println("running")
	defer log.Println("finished")

	var posts []*twitter.Post
	b, err := os.ReadFile(cfg.Input)
	if err != nil {
		return fmt.Errorf("couldn't read tweets from file: %w", err)
	}
	if err := gocsv.UnmarshalBytes(b, &posts); err != nil {
		return fmt.Errorf("couldn't unmarshal tweets from csv: %w", err)
	}
	if len(posts) < 2 {
		return fmt.Errorf("need at least 2 tweets to compare")
	}

	c := openai.New(&openai.Config{
		Debug: cfg.Debug,
		Model: cfg.Model,
		Host:  cfg.Host,
		Token: cfg.Token,
	})

	var tws []*Tweet
	for _, post := range posts {
		tws = append(tws, &Tweet{
			Score: 1200,

			Comments: post.Comments,
			Retweets: post.Retweets,
			Likes:    post.Likes,
			Views:    post.Views,

			Time: post.Time,
			Text: post.Text,
			Link: fmt.Sprintf("https://x.com/%s/status/%s", post.UserID, post.ID),
		})
	}

	iterations := cfg.Iterations
	if iterations < 1 {
		iterations = 1
	}

	// Match tweets against each other
	var exit bool
	var count int
	for i := 0; i < iterations; i++ {
		if exit {
			break
		}
		for _, a := range tws {
			count++
			log.Printf("ai %d/%d\n", count, len(tws)*iterations)

			// Choose a random tweet to compare against
			var b *Tweet
			for {
				b = tws[rand.Intn(len(posts))]
				if b.Link != a.Link {
					break
				}
			}

			resp, err := c.ChatCompletion(ctx, cfg.Prompt+"\n\nTWEET 1: "+a.Link+"\n\nTWEET 2: "+b.Link)
			if err != nil {
				if ctx.Err() != nil {
					exit = true
					break
				}
				return err
			}
			match := numberRegex.FindString(resp)
			if match == "" {
				log.Println("no number found in response")
				log.Println(resp)
				continue
			}
			n, err := strconv.Atoi(match)
			if err != nil {
				log.Println("error parsing number from response")
				log.Println(resp)
				continue
			}
			if n != 1 && n != 2 {
				log.Println("invalid number found in response")
				log.Println(resp)
				continue
			}
			scoreA := 1.0
			scoreB := 0.0
			if n == 2 {
				scoreA = 0.0
				scoreB = 1.0
			}

			newRatingA, newRatingB := updateEloRatings(float64(a.Score), float64(b.Score), scoreA, scoreB)
			a.Score = int(newRatingA)
			b.Score = int(newRatingB)
		}
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
	if cfg.Output != "" {
		if err := os.WriteFile(cfg.Output, data, 0644); err != nil {
			return fmt.Errorf("couldn't write tweets to file: %w", err)
		}
		fmt.Println("created file:", cfg.Output)
	} else {
		fmt.Println(string(data))
	}
	return nil
}

var numberRegex = regexp.MustCompile(`\d+`)

// Constants for the K-factor
const K = 32

// Function to calculate the expected score
func expectedScore(ratingA, ratingB float64) float64 {
	return 1.0 / (1.0 + math.Pow(10, (ratingB-ratingA)/400))
}

// Function to update Elo ratings
func updateEloRatings(ratingA, ratingB, scoreA, scoreB float64) (float64, float64) {
	expectedA := expectedScore(ratingA, ratingB)
	expectedB := expectedScore(ratingB, ratingA)

	newRatingA := ratingA + K*(scoreA-expectedA)
	newRatingB := ratingB + K*(scoreB-expectedB)

	return newRatingA, newRatingB
}
