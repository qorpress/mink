package main

import (
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
)

const (
	TIME_START = "start"
)

type PageResponse struct {
	Url        string
	StatusCode int
	Data       []byte
	Depth      int
	Duration   time.Duration
	Headers    *http.Header
}

type Scraper struct {
	ID          int32
	MaxDepth    int
	Website     string
	Recursively bool
	PrintLogs   bool
	Async       bool
	waitGroup   sync.WaitGroup
	stats       map[string]*PageStats
	mutex       *sync.Mutex
}

func prepareAllowedDomain(requestURL string) ([]string, error) {
	u, err := url.ParseRequestURI(requestURL)
	if err != nil {
		return nil, err
	}
	hostname := u.Hostname()
	domain := strings.TrimLeft(hostname, "wwww.")
	return []string{
		domain,
		"www." + domain,
		"http://" + domain,
		"https://" + domain,
		"http://www." + domain,
		"https://www." + domain,
	}, nil
}

func (s *Scraper) Log(v ...interface{}) {
	if s.PrintLogs {
		log.Print("Scraper ", "#", s.ID, " ", v)
	}
}

func (s *Scraper) Scrape() error {
	s.Log("About to scrap", s.Website)

	//c := colly.NewCollector(colly.Debugger(&debug.LogDebugger{}))
	c := colly.NewCollector()
	extensions.RandomUserAgent(c)

	c.MaxDepth = s.MaxDepth
	c.Async = s.Async
	allowedDomains, err := prepareAllowedDomain(s.Website)
	if err != nil {
		s.Log("Failed to prepare allowed domains.", err)
		return err
	}
	c.AllowedDomains = allowedDomains

	c.OnRequest(func(r *colly.Request) {
		r.Ctx.Put(TIME_START, time.Now())
	})

	c.OnResponse(func(r *colly.Response) {
		s.Log("Received response from", r.Request.URL.String())
		p := &PageResponse{
			Url:        r.Request.URL.String(),
			Data:       r.Body,
			StatusCode: r.StatusCode,
			Depth:      r.Request.Depth,
			Headers:    r.Headers,
		}
		start := r.Ctx.GetAny(TIME_START)
		if start != nil {
			duration := time.Now().Sub(start.(time.Time))
			p.Duration = duration
		}

		s.waitGroup.Add(1)
		go s.processPage(p)
	})

	// Find and visit all links
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")
		s.Log("visiting: ", link)
		if err := e.Request.Visit(link); err != nil {
			if err != colly.ErrAlreadyVisited {
				s.Log("error while linking: ", err.Error())
			}
		}
	})

	// Start the scrape
	if err := c.Visit(s.Website); err != nil {
		s.Log("error while visiting:", err.Error())
	}

	s.Log("Waiting for the scape to finish...")
	c.Wait()

	s.Log("Waiting for the page processing to finish...")
	s.waitGroup.Wait()

	return nil
}

func (s *Scraper) Report() []*PageStats {
	s.Log("Reporting stats count", len(s.stats))
	result := make([]*PageStats, 0, len(s.stats))
	for _, v := range s.stats {
		result = append(result, v)
	}
	return result
}
