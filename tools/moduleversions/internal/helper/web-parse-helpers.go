package helper

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type ModuleInfo struct {
	Channels  map[string]string
	URL       string
	FetchTime time.Time
}

func DhSiteVers(url, webChannel string) error {
	info := &ModuleInfo{
		Channels:  map[string]string{},
		URL:       url,
		FetchTime: time.Now(),
	}

	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Println(err)
		return err
	}

	doc.Find(".submenu-item").Each(func(i int, s *goquery.Selection) {
		channel := s.Find(".submenu-item-channel").Text()
		channel = strings.TrimSpace(channel)
		channel = GetChannel(channel)

		version := s.Find(".submenu-item-release").Text()
		version = GetSemVer(version)
		// version = strings.TrimSpace(version)

		if channel != "" && version != "" {
			info.Channels[channel] = version
		}
	})

	fmt.Println("Infor from ", url)
	// fmt.Println(info)
	fmt.Println("Channel: ", webChannel, "version: ", info.Channels[webChannel])
	return nil
}
