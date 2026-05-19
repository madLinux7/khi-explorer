package downloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/madLinux7/khi-explorer/client"
	"github.com/madLinux7/khi-explorer/config"
)

type DownloadProgress struct {
	Total     int
	Completed int
	Current   string
}

type Downloader struct {
	Client     *client.Client
	Config     config.Config
	ProgressCB func(DownloadProgress)
}

func NewDownloader(c *client.Client, cfg config.Config) *Downloader {
	return &Downloader{
		Client: c,
		Config: cfg,
	}
}

func (d *Downloader) DownloadSong(song client.Song, albumTitle string) error {
	return d.DownloadSongWithCallback(song, albumTitle, nil)
}

func (d *Downloader) DownloadSongWithCallback(song client.Song, albumTitle string, cb func(DownloadProgress)) error {
	downloadURL, err := d.Client.GetDownloadURL(song.URL, d.Config.Format)
	if err != nil {
		return err
	}

	if downloadURL == "" {
		return fmt.Errorf("could not find download URL for %s", song.Title)
	}

	if cb != nil {
		cb(DownloadProgress{Total: 1, Completed: 0, Current: song.Title})
	}

	// Prepare path
	downloadPath := expandHome(d.Config.DownloadPath)
	safeAlbumTitle := sanitizeFilename(albumTitle)
	albumPath := filepath.Join(downloadPath, safeAlbumTitle)
	if err := os.MkdirAll(albumPath, 0755); err != nil {
		return err
	}

	// Determine filename
	ext := ".mp3"
	if strings.HasSuffix(strings.ToLower(downloadURL), ".flac") {
		ext = ".flac"
	}

	filename := fmt.Sprintf("%s - %s%s", song.Number, song.Title, ext)
	filename = sanitizeFilename(filename)
	filePath := filepath.Join(albumPath, filename)

	err = d.downloadFile(downloadURL, filePath)
	if err == nil && cb != nil {
		cb(DownloadProgress{Total: 1, Completed: 1, Current: song.Title})
	}
	return err
}

func (d *Downloader) DownloadAlbum(albumURL string) error {
	return d.DownloadAlbumWithCallback(albumURL, nil)
}

func (d *Downloader) DownloadAlbumWithCallback(albumURL string, cb func(DownloadProgress)) error {
	details, err := d.Client.GetAlbumDetails(albumURL)
	if err != nil {
		return err
	}

	total := len(details.Songs)
	for i, song := range details.Songs {
		if cb != nil {
			cb(DownloadProgress{Total: total, Completed: i, Current: song.Title})
		}
		if err := d.DownloadSongWithCallback(song, details.Title, nil); err != nil {
			// Log error but continue
		}
	}

	if cb != nil {
		cb(DownloadProgress{Total: total, Completed: total, Current: "Done"})
	}

	return nil
}

func (d *Downloader) downloadFile(url string, path string) error {
	req, err := fhttp.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	d.Client.SetBrowserHeaders(req)
	req.Header.Set("Referer", "https://downloads.khinsider.com/")

	resp, err := d.Client.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")
	return strings.TrimSpace(name)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
