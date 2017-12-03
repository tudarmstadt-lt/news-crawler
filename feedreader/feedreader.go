package feedreader

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/SlyMarbo/rss"
	"gopkg.in/cheggaaa/pb.v1"
)

const (
	userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:53.0) Gecko/20100101 Firefox/53.0"
)

// Feed represent an RSS/Atom feed
type Feed struct {
	URL   string      `json:"url"`
	Items []*FeedItem `json:"items"`
}

// FeedItem stores info of feed entry
type FeedItem struct {
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
	GUID      string    `json:"guid"`
}

type FeedReader struct {
	Sources     []string
	Feeds       []Feed
	FailedFeeds []string
	Day         *time.Time
	Verbose     bool
}

// New creates a feedreader
func New(feedsFile string) (FeedReader, error) {
	feedreader := FeedReader{}
	err := feedreader.loadSources(feedsFile)
	return feedreader, err
}

// Fetch feed items
func (fr *FeedReader) Fetch() error {
	wg := sync.WaitGroup{}
	queue := make(chan string)
	errURLChan := make(chan string)
	feedChan := make(chan *Feed)
	count := len(fr.Sources)
	bar := pb.StartNew(count)

	fr.createWorker(&wg, queue, feedChan, errURLChan)
	go fr.fillWorker(queue)

	feeds := make([]Feed, 0)
	failedFeeds := make([]string, 0)
	for i := 0; i < count; i++ {
		select {
		case feed := <-feedChan:
			feeds = append(feeds, *feed)
		case url := <-errURLChan:
			failedFeeds = append(failedFeeds, url)
		}
		bar.Increment()
	}
	bar.Finish()

	fr.Feeds = feeds
	fr.FailedFeeds = failedFeeds

	return nil
}

func (fr *FeedReader) createWorker(wg *sync.WaitGroup, queue chan string, feedChan chan *Feed, errURLChan chan string) {
	concurrencyLimit := 100
	for worker := 0; worker < concurrencyLimit; worker++ {
		wg.Add(1)
		go fr.runWorker(wg, queue, feedChan, errURLChan)
	}
}

func (fr *FeedReader) fillWorker(queue chan string) {
	for _, url := range fr.Sources {
		queue <- url
	}
	close(queue)
}

func (fr *FeedReader) runWorker(wg *sync.WaitGroup, queue chan string, feedChan chan *Feed, errURLChan chan string) {
	defer wg.Done()
	for url := range queue {
		items, err := fr.fetchFeed(url, fr.Day)
		if err != nil {
			if fr.Verbose {
				log.Printf("Failed to fetch feed %s %s", url, err)
			}
			errURLChan <- url
			continue
		}

		feed := Feed{
			URL:   url,
			Items: items,
		}
		feedChan <- &feed
	}
}

func (*FeedReader) fetchFeedURL(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	timeout := time.Duration(30 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}

	return client.Do(req)
}

func (fr *FeedReader) fetchFeed(url string, dayTime *time.Time) ([]*FeedItem, error) {
	feed, err := rss.FetchByFunc(fr.fetchFeedURL, url)
	if err != nil {
		return nil, err
	}

	day := dayTime.Format("2-1-2006")
	items := make([]*FeedItem, 0)
	for _, item := range feed.Items {
		// only accept feed items from today
		if item.Date.Format("2-1-2006") != day {
			continue
		}

		newItem := FeedItem{
			Title:     item.Title,
			Content:   item.Content,
			URL:       item.Link,
			Published: item.Date,
			GUID:      item.ID,
		}

		err := newItem.validate()
		if err != nil {
			continue
		}

		items = append(items, &newItem)
	}

	return items, nil
}

func (item *FeedItem) validate() error {
	if item.URL == "" {
		return fmt.Errorf("Feed item contains no url: %s", item)
	}

	if item.GUID == "" {
		item.GUID = item.URL
	}

	return nil
}
