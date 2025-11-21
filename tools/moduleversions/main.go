package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	moduleName = "virtualization"
	baseURL    = "https://releases.deckhouse.io"
)

var (
	feURL  = baseURL + "/fe"
	eeURL  = baseURL + "/ee"
	ceURL  = baseURL + "/ce"
	sePlus = baseURL + "/se-plus"
)

type ChannelVersion struct {
	Edition string
	Number  string
	Channel string
}

type ModuleInfo struct {
	Module   string
	Versions []ChannelVersion
}

func (v ModuleInfo) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Module: %s\n", v.Module))
	for _, version := range v.Versions {
		b.WriteString(fmt.Sprintf("%-7s %s %s\n", version.Edition, version.Channel, version.Number))
	}
	return b.String()
}

func getCompareVersions(url, channel, version string) (error, bool) {
	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return err, false
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Println(err)
		return err, false
	}

	var (
		index      int
		webVersion string
	)

	switch channel {
	case "alpha":
		index = 1
	case "beta":
		index = 2
	case "early-access":
		index = 3
	case "stable":
		index = 4
	case "rock-solid":
		index = 5
	default:
		return nil, false
	}

	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		if strings.Contains(s.Text(), moduleName) {
			cells := s.Find("td")
			if cells.Length() == 6 {
				webVersion = strings.TrimSpace(cells.Eq(index).Text())
			}
		}
	})

	if webVersion != version {
		return fmt.Errorf("version mismatch: %s != %s", webVersion, version), false
	}

	return nil, true
}

func getSemVer(version string) string {
	if strings.HasPrefix(version, "v") {
		version = strings.TrimPrefix(version, "v")
		return version
	} else {
		return version
	}
}

func getWebVersions(urlList []string, channel, version string) (error, bool) {
	var (
		match bool
		err   error
	)

	versions := ModuleInfo{
		Module:   moduleName,
		Versions: []ChannelVersion{},
	}

	for _, url := range urlList {
		if err, match = getCompareVersions(url, channel, version); err != nil {
			log.Printf("Error fetching %-7s on the channel %s: %v", strings.Split(url, "/")[3], cases.Title(language.Und).String(channel), err)
			continue
		}

		if !match {
			log.Printf("Error fetching %-7s on the channel %s: %v", strings.Split(url, "/")[3], cases.Title(language.Und).String(channel), err)
			continue
		} else {
			versions.Versions = append(versions.Versions, ChannelVersion{
				Edition: strings.Split(url, "/")[3],
				Channel: channel,
				Number:  version,
			})
		}
	}

	if match {
		fmt.Println("Version is valid")
		fmt.Println(versions)
		return nil, true
	}
	return nil, false
}

func main() {
	var (
		match bool
		count int
		err   error
	)

	flag.String("channel", "", "alpha, beta, early-access, stable, rock-solid")
	flag.String("version", "", "version like 1.1.2 or v1.2")
	flag.IntVar(&count, "count", 10, "try count, default 10")
	flag.Parse()

	channel := flag.Lookup("channel").Value.String()
	version := getSemVer(flag.Lookup("version").Value.String())

	if channel == "" || version == "" {
		flag.Usage()
		return
	}

	channel = strings.ReplaceAll(strings.ToLower(channel), " ", "-")

	urlList := []string{feURL, eeURL, ceURL, sePlus}

	for i := 0; i < count; i++ {
		err, match = getWebVersions(urlList, channel, version)
		if err != nil {
			log.Println(err)
			continue
		}
		if match {
			fmt.Println("Version is valid")
			break
		}

		fmt.Println("Wait 10 seconds before next try")
		time.Sleep(10 * time.Second)
		break
	}
	if !match {
		log.Fatal("Version is not valid")
	}
}
