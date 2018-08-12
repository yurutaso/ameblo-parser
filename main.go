package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	AMEBA_DOMAIN                  string = `https://ameblo.jp`
	AMEBA_STAT_DOMAIN             string = `https://stat.ameba.jp`
	AMEBA_ENTRYLIST_INDEX_PATTERN string = "%s/%s/entrylist.html"
	AMEBA_ENTRYLIST_PATTERN       string = "%s/%s/entrylist-%d.html"
	IMAGE_DIR                     string = "ameblo"
)

type Ameblo struct {
	Author  string
	Entries []AmebloEntry
}

type AmebloEntry struct {
	u      string
	doc    *goquery.Document
	title  string
	date   string
	images []string
}

func (entry *AmebloEntry) Images() ([]string, error) {
	if entry.images == nil {
		if err := entry.getImages(); err != nil {
			return nil, err
		}
	}
	return entry.images, nil
}

func (entry *AmebloEntry) Date() (string, error) {
	if entry.date == `` {
		if err := entry.getDate(); err != nil {
			return ``, err
		}
	}
	return entry.date, nil
}

func (entry *AmebloEntry) Title() (string, error) {
	if entry.title == `` {
		if err := entry.getTitle(); err != nil {
			return ``, err
		}
	}
	return entry.title, nil
}

func (entry *AmebloEntry) checkDocument() error {
	if entry.doc == nil {
		return entry.getDocument()
	}
	return nil
}

func (entry *AmebloEntry) getDocument() error {
	res, err := http.Get(AMEBA_DOMAIN + entry.u)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		return err
	}
	entry.doc = doc
	return nil
}

func (entry *AmebloEntry) getTitle() error {
	if err := entry.checkDocument(); err != nil {
		return err
	}
	entry.title = entry.doc.Find(`h1.skin-entryTitle`).First().Text()
	return nil
}

func (entry *AmebloEntry) getDate() error {
	if err := entry.checkDocument(); err != nil {
		return err
	}
	/*
		pubdate, exists := entry.doc.Find(`p.skin-entryPubdate>time`).Attr(`datetime`)
		if !exists {
			return fmt.Errorf(`cannot find publish datetime`)
		}
		entry.date = pubdate
	*/
	s := entry.doc.Find(`p.skin-entryPubdate>time`)
	s.Find(`span`).Remove()
	entry.date = strings.Replace(strings.Replace(s.Text(), ` `, `_`, -1), `:`, `-`, -1)
	return nil
}

func (entry *AmebloEntry) getImages() error {
	if err := entry.checkDocument(); err != nil {
		return err
	}

	images := make([]string, 0)
	//mainbody := entry.doc.Find(`article.skin-entry`).First()
	mainbody := entry.doc.Find(`div.skin-entryBody`).First()
	mainbody.Find(`img`).Each(func(i int, s *goquery.Selection) {
		image, exists := s.Attr(`src`)
		if exists {
			// Skip dummy contents with file:// protocol
			if image[0:4] != `file` {
				images = append(images, image)
			}
		}
	})

	/*
		 // Recent images have link with a>img tag, but above expression is enough.
		mainbody.Find(`a>img`).Each(func(i int, s *goquery.Selection) {
			image, exists := s.Attr(`src`)
			if exists {
				images = append(images, image)
			}
		})
	*/
	entry.images = images
	return nil
}

func GetEntries(author string) ([]*AmebloEntry, error) {
	num, err := getNumPages(author)
	if err != nil {
		return nil, err
	}

	entries := make([]*AmebloEntry, 0)
	for page := 0; page < num; page++ {
		u := fmt.Sprintf(AMEBA_ENTRYLIST_PATTERN, AMEBA_DOMAIN, author, page+1)
		res, err := http.Get(u)
		if err != nil {
			return nil, err
		}

		doc, err := goquery.NewDocumentFromResponse(res)
		if err != nil {
			return nil, err
		}

		doc.Find(`ul.skin-archiveList>li.skin-borderQuiet`).Each(func(i int, s *goquery.Selection) {
			entry, exists := s.Find(`h2>a`).First().Attr(`href`)
			if !exists {
				fmt.Println("cannot find link in %dth loop", i)
			}
			entries = append(entries, &AmebloEntry{u: entry, doc: nil})
		})
	}
	return entries, nil
}

func getNumPages(author string) (int, error) {
	u := fmt.Sprintf(AMEBA_ENTRYLIST_INDEX_PATTERN, AMEBA_DOMAIN, author)
	res, err := http.Get(u)
	if err != nil {
		return -1, err
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromResponse(res)
	if err != nil {
		return -1, err
	}
	pageEnd := doc.Find(`li>a.skin-paginationEnd`).First()
	u, exists := pageEnd.Attr(`href`)
	if !exists {
		return -1, fmt.Errorf(`can not find href in the tag`)
	}

	usplit := strings.Split(u, `-`)
	if len(usplit) != 2 {
		return -1, fmt.Errorf("can not split the url %s", u)
	}
	num, err := strconv.Atoi(strings.TrimSuffix(usplit[1], `.html`))
	if err != nil {
		return -1, err
	}
	return num, nil
}

func MkdirIfNotExists(dirname string, perm os.FileMode) error {
	if _, err := os.Stat(dirname); err == nil {
		fmt.Printf("Directory %s exists\n", dirname)
		return nil
	}
	return os.Mkdir(dirname, perm)
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal(fmt.Errorf(`You must set only one argument (= author name of the blog)`))
	}
	author := os.Args[1]

	entries, err := GetEntries(author)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d entries found.\n", len(entries))

	outdir, err := filepath.Abs(filepath.Join(IMAGE_DIR, author))
	if err != nil {
		log.Fatal(err)
	}

	// Make output directory
	if err := MkdirIfNotExists(outdir, 0755); err != nil {
		log.Fatal(err)
	}

	// Save images
	for _, entry := range entries {
		// get image urls
		images, err := entry.Images()
		if err != nil {
			log.Println(err)
			continue
		}

		// get information
		title, err := entry.Title()
		if err != nil {
			log.Println(err)
			continue
		}

		date, err := entry.Date()
		if err != nil {
			log.Println(err)
			continue
		}

		fmt.Printf("URL: %s, Title: %s, Date: %s", entry.u, title, date)
		fmt.Println(images)

		// Create output directory.
		year := date[0:4]
		month := date[5:7]
		if err := MkdirIfNotExists(filepath.Join(outdir, year), 0755); err != nil {
			log.Fatal(err)
		}
		if err := MkdirIfNotExists(filepath.Join(outdir, year, month), 0755); err != nil {
			log.Fatal(err)
		}

		dirname := filepath.Join(outdir, year, month, date)
		if err := MkdirIfNotExists(dirname, 0755); err != nil {
			log.Fatal(err)
		}

		for i, image := range images {
			name := fmt.Sprintf("%s_%d.jpg", date, i)

			// check if file exists
			fname := filepath.Join(dirname, name)
			if _, err := os.Stat(fname); err == nil {
				fmt.Printf("File %s exists\n", fname)
				continue
			}

			out, err := os.Create(fname)
			if err != nil {
				log.Fatal(err)
			}
			defer out.Close()

			res, err := http.Get(image)
			if err != nil {
				log.Fatal(err)
			}
			defer res.Body.Close()

			_, err = io.Copy(out, res.Body)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}
