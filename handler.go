package main

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/feeds"
)

func worker(jobs <-chan url.URL, results chan<- string) {
	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	for j := range jobs {
		resp, err := client.Get(j.String())
		if err != nil {
			results <- ""
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			results <- ""
			continue
		}

		results <- string(body)
	}
}

//RSSMergeHandler configures borked handlers with timeouts and concurrency settings
func RSSMergeHandler(w http.ResponseWriter, r *http.Request) {
	sourceURLs := r.URL.Query()["source"]

	if sourceURLs == nil || len(sourceURLs) == 0 {
		http.Error(w, "missing feed source", http.StatusBadRequest)
		return
	}

	sourceURL, err := url.Parse(sourceURLs[0])
	if err != nil {
		http.Error(w, "invalid source URL", http.StatusBadRequest)
		return
	}

	feedURLs, err := FetchList(sourceURL)
	if err != nil {
		http.Error(w, "failed to fetch source list", http.StatusBadRequest)
		return
	}

	isDay := false
	if len(r.URL.Query()["day"]) != 0 {
		isDay = true
	}

	isConcat := false
	if len(r.URL.Query()["concat"]) != 0 {
		isConcat = true
	}

	feedJobs := make(chan url.URL, len(feedURLs))
	feedResults := make(chan string, len(feedURLs))

	for i := 1; i <= 20; i++ {
		go worker(feedJobs, feedResults)
	}

	for _, v := range feedURLs {
		feedJobs <- v
	}
	close(feedJobs)

	var rawFeeds []string
	for i := 1; i <= len(feedURLs); i++ {
		rawFeeds = append(rawFeeds, <-feedResults)
	}

	mergedFeed := Merge(rawFeeds, isConcat)
	mergedFeed.Link.Href = sourceURL.String()

	if isDay {
		var filteredItems []*feeds.Item
		for _, v := range mergedFeed.Items {
			if time.Since(v.Created) < time.Since(mergedFeed.Items[0].Created.Add(time.Hour*-24)) {
				filteredItems = append(filteredItems, v)
			}
		}
		mergedFeed.Items = filteredItems
	}

	rss, err := mergedFeed.ToRss()
	if err != nil {
		http.Error(w, "failed to build merged feed", http.StatusBadRequest)
		return
	}

	w.Write([]byte(rss))
}
