package twai

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
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
	Debug       bool
	Concurrency int
	Input       string
	Output      string
	Prompt      string
	Model       string
	Host        string
	Token       string
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

	// Concurrency settings
	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}

	var idx int
	var lck sync.Mutex

	var tws []*Tweet

	// Launch concurrent ai completions
	concurrent(ctx, concurrency,
		func() (*twitter.Post, bool) {
			if idx >= len(posts) {
				return nil, false
			}
			post := posts[idx]
			log.Printf("ai: tweet %d/%d\n", idx+1, len(posts))
			idx++
			return post, true
		},
		func(post *twitter.Post) error {
			// Ask for a score
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
			lck.Lock()
			defer lck.Unlock()
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
			return nil
		},
	)

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
	Debug       bool
	Concurrency int
	Input       string
	Output      string
	Iterations  int
	Model       string
	Host        string
	Prompt      string
	Token       string
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

	// Concurrency settings
	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}

	var idx int
	var currIteration int
	var lck sync.Mutex

	// Run concurrent ai completions
	concurrent(ctx, concurrency,
		func() (*Tweet, bool) {
			if idx >= len(tws) {
				idx = 0
				currIteration++
				if currIteration >= iterations {
					return nil, false
				}
			}
			tw := tws[idx]
			log.Printf("ai: iteration %d/%d, tweet %d/%d\n", currIteration+1, iterations, idx+1, len(tws))
			idx++
			return tw, true
		},
		func(a *Tweet) error {
			// Choose a random tweet to compare against
			var b *Tweet
			for {
				b = tws[rand.Intn(len(posts))]
				if b.Link != a.Link {
					break
				}
			}

			// Make the comparison
			resp, err := c.ChatCompletion(ctx, cfg.Prompt+"\n\nTWEET 1: "+a.Link+"\n\nTWEET 2: "+b.Link)
			if err != nil {
				return err
			}
			match := numberRegex.FindString(resp)
			if match == "" {
				log.Println(resp)
				return errors.New("no number found in response")
			}
			n, err := strconv.Atoi(match)
			if err != nil {
				log.Println(resp)
				return fmt.Errorf("error parsing number from response: %w", err)
			}
			if n != 1 && n != 2 {
				log.Println(resp)
				return errors.New("invalid number found in response")
			}
			scoreA := 1.0
			scoreB := 0.0
			if n == 2 {
				scoreA = 0.0
				scoreB = 1.0
			}

			// Update Elo ratings
			lck.Lock()
			defer lck.Unlock()
			newRatingA, newRatingB := updateEloRatings(float64(a.Score), float64(b.Score), scoreA, scoreB)
			a.Score = int(newRatingA)
			b.Score = int(newRatingB)
			return nil
		},
	)

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

// Generic concurrent function
func concurrent[T any](ctx context.Context, n int, next func() (T, bool), fn func(T) error) {
	errC := make(chan error, n)
	defer close(errC)
	for i := 0; i < n; i++ {
		errC <- nil
	}
	var wg sync.WaitGroup

	var nErr int
	for {
		var err error
		select {
		case <-ctx.Done():
			log.Println("context cancelled")
		case err = <-errC:
		}
		if ctx.Err() != nil {
			break
		}
		if err != nil {
			nErr += 1
		} else {
			nErr = 0
		}

		// Check exit conditions
		if nErr > 10 {
			log.Println("too many consecutive errors")
			break
		}

		// Get next value
		v, ok := next()
		if !ok {
			break
		}

		// Launch job in a goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn(v)
			if err != nil {
				log.Println(err)
			}
			errC <- err
		}()
	}
	wg.Wait()
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
