package twitter

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

type Post struct {
	ID            string    `json:"id"`
	Text          string    `json:"text"`
	Time          time.Time `json:"time"`
	UserID        string    `json:"user_id"`
	UserName      string    `json:"user_name"`
	UserFollowers int       `json:"user_followers"`
	Comments      int       `json:"comments"`
	Retweets      int       `json:"retweets"`
	Likes         int       `json:"likes"`
	Views         int       `json:"views"`
}

func (c *Browser) Posts(parent context.Context, page string, n int, withFollowers bool) ([]*Post, error) {
	// Create a new tab based on client context
	ctx, cancel := chromedp.NewContext(c.browserContext)
	defer cancel()

	go func() {
		select {
		case <-parent.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	// Use home page if no page is provided
	if page == "" {
		page = "home"
	}

	// Navigate to the page
	u := fmt.Sprintf("https://x.com/%s", page)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(u),
		chromedp.WaitVisible("body", chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("twitter: couldn't navigate to url: %w", err)
	}

	// Wait for the posts to load
	if err := chromedp.Run(ctx,
		chromedp.WaitVisible("article time[datetime]", chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("twitter: couldn't wait for posts: %w", err)
	}

	ids := map[string]struct{}{}
	var posts []*Post
	followers := map[string]int{}
	var noPost int
	for {
		select {
		case <-ctx.Done():
			return posts, nil
		case <-time.After(2 * time.Second):
		}
		currentPosts, err := getPosts(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("twitter: couldn't get posts: %w", err)
		}
		for _, post := range currentPosts {
			ids[post.ID] = struct{}{}

			// Obtain the user stats
			if withFollowers {
				f, ok := followers[post.UserID]
				if !ok {
					candidate, err := getFolowers(ctx, post.UserID)
					if err != nil {
						return nil, fmt.Errorf("twitter: couldn't get followers: %w", err)
					}
					f = candidate
				}
				post.UserFollowers = f
			}

			// Append the post
			posts = append(posts, post)
		}

		// Check if no more posts are found
		if len(currentPosts) == 0 {
			noPost++
			if noPost > 2 {
				log.Println("no more posts found, reached the end")
				return posts, nil
			}
		} else {
			noPost = 0
		}
		if len(posts) >= n {
			return posts[:n], nil
		}
		log.Printf("tweet %d/%d\n", len(posts), n)

		// Scroll down
		if err := scrollDown(ctx); err != nil {
			return nil, fmt.Errorf("twitter: couldn't scroll down: %w", err)
		}
	}
}

func getFolowers(ctx context.Context, userID string) (int, error) {
	// Scroll to the user
	avatar := fmt.Sprintf(`div[data-testid="User-Name"] a[href="/%s"]`, userID)
	if err := chromedp.Run(ctx, chromedp.ScrollIntoView(avatar)); err != nil {
		return 0, fmt.Errorf("twitter: couldn't scroll into view: %w", err)
	}

	// Move the mouse to the user
	if err := mouseMove(ctx, avatar); err != nil {
		return 0, err
	}

	// Wait for the user stats to appear
	stats := fmt.Sprintf(`div[data-testid="HoverCard"] a[href="/%s/verified_followers"] span`, userID)
	if err := chromedp.Run(ctx, chromedp.WaitVisible(stats, chromedp.ByQuery)); err != nil {
		return 0, fmt.Errorf("twitter: couldn't wait for user stats: %w", err)
	}

	// Load the document
	doc, err := getHTML(ctx, "body")
	if err != nil {
		return 0, err
	}

	// Get the followers
	v := doc.Find(stats).First().Text()
	followers, err := toNumber(v)
	if err != nil {
		log.Printf("error parsing followers: %v\n", err)
	}

	// Move the mouse up
	if err := chromedp.Run(ctx, chromedp.MouseEvent(input.MouseMoved, 10, 10)); err != nil {
		return 0, fmt.Errorf("twitter: couldn't move mouse: %w", err)
	}

	// Wait for the user stats to disappear
	for {
		var isNotVisible bool
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(`document.querySelector('`+stats+`') === null || document.querySelector('`+stats+`').offsetParent === null`, &isNotVisible),
		); err != nil {
			return 0, fmt.Errorf("twitter: couldn't check if stats are not visible: %w", err)
		}
		if isNotVisible {
			break
		}
		select {
		case <-ctx.Done():
			return 0, nil
		case <-time.After(100 * time.Millisecond):
		}
	}
	return followers, nil
}

func getPosts(ctx context.Context, ids map[string]struct{}) ([]*Post, error) {
	// Get the main document
	doc, err := getHTML(ctx, "main")
	if err != nil {
		return nil, err
	}

	// Get posts
	var posts []*Post
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		var p Post
		// Search time
		timeNode := s.Find(`time[datetime]`).First()
		ts, ok := timeNode.Attr("datetime")
		if !ok {
			// Skip if no time is found
			return
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			log.Printf("error parsing time %s: %v\n", ts, err)
			return
		}
		p.Time = t

		// Search ID
		link, ok := timeNode.Parent().Attr("href")
		if !ok {
			return
		}
		if link == "" {
			log.Println("empty link")
			return
		}
		parts := strings.Split(link, "/")
		id := parts[len(parts)-1]
		if _, ok := ids[id]; ok {
			return
		}

		p.ID = id

		// Search user ID
		p.UserID = strings.TrimPrefix(strings.TrimSpace(s.Find(`div[data-testid="User-Name"] a > div > span`).First().Text()), "@")

		// Search user name
		p.UserName = strings.TrimSpace(s.Find(`div[data-testid="User-Name"] a`).First().Text())

		// Search post text
		text := strings.TrimSpace(s.Find(`div[data-testid="tweetText"]`).First().Text())
		p.Text = strings.ReplaceAll(text, "\n", " ")

		// Search stats (comments, retweets, likes, views)
		s.Find(`span[data-testid="app-text-transition-container"]`).Each(func(i int, s *goquery.Selection) {
			v := s.Text()
			n, err := toNumber(v)
			if err != nil {
				log.Printf("error parsing number: %v\n", err)
				return
			}
			switch i {
			case 0:
				p.Comments = n
			case 1:
				p.Retweets = n
			case 2:
				p.Likes = n
			case 3:
				p.Views = n
			}
		})
		if p.ID == "" || p.UserID != "" || p.Text != "" {
			posts = append(posts, &p)
		}
	})
	return posts, nil
}

func toNumber(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, ",", "")
	mul := 1
	switch {
	case strings.HasSuffix(s, "K"):
		mul = 1000
	case strings.HasSuffix(s, "M"):
		mul = 1000000
	case strings.HasSuffix(s, "B"):
		mul = 1000000000
	}
	var views int
	if v := decimalRegex.FindString(s); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("error parsing views %s: %w", s, err)
		}
		views = int(f * float64(mul))
	} else if v := numberRegex.FindString(s); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("error parsing views %s: %w", s, err)
		}
		views = n * mul
	} else {
		return 0, fmt.Errorf("error parsing views %s", s)
	}
	return views, nil
}

var numberRegex = regexp.MustCompile(`\d+`)
var decimalRegex = regexp.MustCompile(`\d+.\d+`)

func getHTML(ctx context.Context, sel string) (*goquery.Document, error) {
	// Obtain the document
	var html string
	if err := chromedp.Run(ctx,
		chromedp.OuterHTML(sel, &html),
	); err != nil {
		return nil, fmt.Errorf("twitter: couldn't get html %s: %w", sel, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("twitter: couldn't parse doc %s: %w", sel, err)
	}
	return doc, nil
}

func scrollDown(ctx context.Context) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(`window.scrollTo(0, document.body.scrollHeight)`, nil).Do(ctx)
		}),
	)
}

func mouseMove(ctx context.Context, sel string) error {
	// Variables to store the x and y coordinates
	var x, y float64

	// First, get the x and y coordinates of the center of the div
	if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
	var rect = document.querySelector('%s').getBoundingClientRect();
	[rect.left + (rect.width / 2), rect.top + (rect.height / 2)]
`, sel), &[]*float64{&x, &y})); err != nil {
		return fmt.Errorf("twitter: couldn't get coordinates: %w", err)
	}

	// Scroll to that position
	if y < 100 {
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`window.scrollTo(%f, %f)`, x, y-100), nil)); err != nil {
			return fmt.Errorf("twitter: couldn't scroll to coordinates: %w", err)
		}
	}

	// Now, move the mouse to the retrieved coordinates
	if err := chromedp.Run(ctx, chromedp.MouseEvent(input.MouseMoved, x, y)); err != nil {
		return fmt.Errorf("twitter: couldn't move mouse: %w", err)
	}
	return nil
}
