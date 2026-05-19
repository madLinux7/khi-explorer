package client

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const BaseURL = "https://downloads.khinsider.com"

type Client struct {
	UserAgent string
	HTTP      tls_client.HttpClient
}

type Album struct {
	Title    string
	URL      string
	Platform string
	Type     string
	Year     string
}

type Song struct {
	Number   string
	Title    string
	URL      string // Page URL
	Duration string
	MP3Size  string
	FLACSize string
}

type AlbumDetails struct {
	Title string
	Songs []Song
}

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0"

func NewClient() *Client {
	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profiles.Chrome_120),
		tls_client.WithRandomTLSExtensionOrder(),
	}

	client, _ := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

	return &Client{
		UserAgent: DefaultUserAgent,
		HTTP:      client,
	}
}

func (c *Client) SetBrowserHeaders(req *fhttp.Request) {
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="110", "Microsoft Edge";v="110"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

func (c *Client) doRequest(url string) (*goquery.Document, error) {
	req, err := fhttp.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	c.SetBrowserHeaders(req)
	req.Header.Set("Referer", BaseURL)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

func (c *Client) Search(query string) ([]Album, error) {
	url := fmt.Sprintf("%s/search?search=%s", BaseURL, strings.ReplaceAll(query, " ", "+"))
	doc, err := c.doRequest(url)
	if err != nil {
		return nil, err
	}

	var albums []Album
	doc.Find("table.albumList tr").Each(func(i int, s *goquery.Selection) {
		if i == 0 { // Skip header
			return
		}
		
		tds := s.Find("td")
		if tds.Length() < 3 {
			return
		}

		titleLink := tds.Eq(1).Find("a")
		album := Album{
			Title:    titleLink.Text(),
			URL:      titleLink.AttrOr("href", ""),
			Platform: tds.Eq(2).Text(),
			Type:     tds.Eq(3).Text(),
			Year:     tds.Eq(4).Text(),
		}
		
		if !strings.HasPrefix(album.URL, "http") {
			album.URL = BaseURL + album.URL
		}
		
		albums = append(albums, album)
	})

	return albums, nil
}

func (c *Client) GetAlbumDetails(albumURL string) (AlbumDetails, error) {
	doc, err := c.doRequest(albumURL)
	if err != nil {
		return AlbumDetails{}, err
	}

	details := AlbumDetails{
		Title: doc.Find("h2").First().Text(),
	}

	// Find column indices dynamically using colspan tracking
	tdIndexMap := make(map[string]int)
	currentTdIdx := 0
	doc.Find("table#songlist tr#songlist_header th").Each(func(i int, s *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(s.Text()))
		colspan := 1
		if cStr, exists := s.Attr("colspan"); exists {
			fmt.Sscanf(cStr, "%d", &colspan)
		}

		if strings.Contains(text, "song name") {
			if colspan > 1 {
				tdIndexMap["duration"] = currentTdIdx + 1
			}
		} else if strings.Contains(text, "mp3") {
			tdIndexMap["mp3"] = currentTdIdx
		} else if strings.Contains(text, "flac") {
			tdIndexMap["flac"] = currentTdIdx
		}

		currentTdIdx += colspan
	})

	doc.Find("table#songlist tr").Each(func(i int, s *goquery.Selection) {
		if s.AttrOr("id", "") == "songlist_header" || s.Find("td").Length() < 4 {
			return
		}

		tds := s.Find("td")
		titleLink := s.Find("td.clickable-row a").First()
		if titleLink.Length() == 0 {
			return
		}

		song := Song{
			Number: strings.TrimSpace(tds.Eq(1).Text()),
			Title:  titleLink.Text(),
			URL:    titleLink.AttrOr("href", ""),
		}

		if idx, ok := tdIndexMap["duration"]; ok && idx < tds.Length() {
			song.Duration = cleanIconText(tds.Eq(idx).Text())
		}
		if idx, ok := tdIndexMap["mp3"]; ok && idx < tds.Length() {
			song.MP3Size = cleanIconText(tds.Eq(idx).Text())
		}
		if idx, ok := tdIndexMap["flac"]; ok && idx < tds.Length() {
			song.FLACSize = cleanIconText(tds.Eq(idx).Text())
		}

		if !strings.HasPrefix(song.URL, "http") {
			song.URL = BaseURL + song.URL
		}

		details.Songs = append(details.Songs, song)
	})

	return details, nil
}

func cleanIconText(text string) string {
	text = strings.TrimSpace(text)
	// Khinsider uses these Material icon names in cells
	text = strings.TrimPrefix(text, "get_app")
	text = strings.TrimSuffix(text, "get_app")
	text = strings.TrimPrefix(text, "playlist_add")
	text = strings.TrimSuffix(text, "playlist_add")
	text = strings.TrimPrefix(text, "play_arrow")
	text = strings.TrimSuffix(text, "play_arrow")
	return strings.TrimSpace(text)
}

func (c *Client) GetDownloadURL(songURL string, format string) (string, error) {
	doc, err := c.doRequest(songURL)
	if err != nil {
		return "", err
	}

	var downloadURL string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		if format == "flac" && strings.HasSuffix(strings.ToLower(href), ".flac") {
			downloadURL = href
		} else if format == "mp3" && strings.HasSuffix(strings.ToLower(href), ".mp3") && downloadURL == "" {
			downloadURL = href
		}
	})

	if downloadURL == "" {
		downloadURL = doc.Find("audio#audio").AttrOr("src", "")
	}

	if downloadURL != "" && !strings.HasPrefix(downloadURL, "http") {
		downloadURL = BaseURL + downloadURL
	}

	return downloadURL, nil
}
