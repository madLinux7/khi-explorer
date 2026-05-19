package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/madLinux7/khi-explorer/client"
	"github.com/madLinux7/khi-explorer/config"
	"github.com/madLinux7/khi-explorer/downloader"
)

// Design System / Colors
const (
	ColorPrimary   = "#D946EF" // Magenta
	ColorSecondary = "#ADBAFF" // Soft Blue
	ColorBorder    = "#7B2FBE" // Purple
	ColorDim       = "#626262" // Grey
	ColorWhite     = "#F8F8F8"
	ColorError     = "#FF0000"
)

var (
	titleStyle    = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color(ColorPrimary)).Bold(true)
	helpStyle     = lipgloss.NewStyle().MarginLeft(2)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
	playingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSecondary)).Bold(true)
	downloadStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSecondary)).Italic(true)
	loadingStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorSecondary))
	appStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(ColorBorder))
	noItemsStyle  = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color(ColorDim))
	plainStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWhite))
)

type state int

const (
	stateSearch state = iota
	stateAlbumList
	stateSongList
	stateLoading
)

type downloadTask struct {
	url        string
	albumTitle string
	isSong     bool
	song       client.Song
}

type mainModel struct {
	state      state
	config     config.Config
	client     *client.Client
	downloader *downloader.Downloader

	searchInput textinput.Model
	albumList   list.Model
	songList    list.Model
	progressBar progress.Model

	currentAlbum client.AlbumDetails
	loadingMsg   string
	err          error

	player       *exec.Cmd
	downloadChan chan downloader.DownloadProgress
	taskChan     chan downloadTask
	nowPlaying   string
	isPaused     bool

	// Download tracking
	downloadInfo downloader.DownloadProgress
	showProgress bool
	queueCount   int

	width  int
	height int
}

func NewMainModel(cfg config.Config) mainModel {
	sip := textinput.New()
	sip.Placeholder = "Search for OST..."
	sip.PromptStyle = plainStyle
	sip.TextStyle = plainStyle.Italic(true)
	sip.Focus()

	// Create custom delegates with bold titles
	d := list.NewDefaultDelegate()
	d.Styles.NormalTitle = d.Styles.NormalTitle.Bold(true).Foreground(lipgloss.Color(ColorSecondary))
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Bold(true).Foreground(lipgloss.Color(ColorPrimary))
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(lipgloss.Color(ColorDim))
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(lipgloss.Color(ColorWhite))

	ali := list.New([]list.Item{}, d, 0, 0)
	ali.SetShowTitle(false)
	ali.SetShowStatusBar(true)
	ali.KeyMap.AcceptWhileFiltering.SetHelp("Enter ↵", "apply filter")
	ali.KeyMap.CancelWhileFiltering.SetHelp("Esc", "cancel")
	ali.KeyMap.NextPage.SetKeys("pgdown", "right")
	ali.KeyMap.PrevPage.SetKeys("pgup", "left")
	ali.KeyMap.Filter.SetKeys()
	ali.KeyMap.ClearFilter.SetKeys()

	sli := list.New([]list.Item{}, d, 0, 0)
	sli.SetShowTitle(false)
	sli.SetShowStatusBar(true)
	sli.KeyMap.AcceptWhileFiltering.SetHelp("Enter ↵", "apply filter")
	sli.KeyMap.CancelWhileFiltering.SetHelp("Esc", "cancel")
	sli.KeyMap.ClearFilter.SetHelp("Esc", "clear filter")
	sli.KeyMap.NextPage.SetKeys("pgdown", "right")
	sli.KeyMap.PrevPage.SetKeys("pgup", "left")
	sli.FilterInput.Prompt = "Filter > "
	sli.FilterInput.PromptStyle = plainStyle
	sli.FilterInput.Cursor.Style = plainStyle
	sli.FilterInput.TextStyle = plainStyle.Italic(true)

	pb := progress.New(progress.WithGradient(ColorBorder, ColorPrimary))

	c := client.NewClient()
	m := mainModel{
		state:        stateSearch,
		config:       cfg,
		client:       c,
		downloader:   downloader.NewDownloader(c, cfg),
		searchInput:  sip,
		albumList:    ali,
		songList:     sli,
		progressBar:  pb,
		downloadChan: make(chan downloader.DownloadProgress, 100),
		taskChan:     make(chan downloadTask, 100),
	}

	// Start the background worker
	go m.downloadWorker()

	return m
}

func (m *mainModel) downloadWorker() {
	for task := range m.taskChan {
		if task.isSong {
			_ = m.downloader.DownloadSongWithCallback(task.song, task.albumTitle, func(p downloader.DownloadProgress) {
				m.downloadChan <- p
			})
		} else {
			_ = m.downloader.DownloadAlbumWithCallback(task.url, func(p downloader.DownloadProgress) {
				m.downloadChan <- p
			})
		}
	}
}

func (m *mainModel) recalcSizes() {
	h, v := appStyle.GetFrameSize()
	innerWidth := m.width - h
	innerHeight := m.height - v

	m.progressBar.Width = innerWidth - 10
	if m.progressBar.Width < 0 {
		m.progressBar.Width = 0
	}

	// Hint rows span 2 lines + 1 line padding for progress bar
	footerHeight := 3

	listH, listV := lipgloss.NewStyle().Margin(1, 2).GetFrameSize()

	listWidth := innerWidth - listH
	if listWidth < 0 {
		listWidth = 0
	}

	// Subtract 2 for the custom header and its trailing newline
	listHeight := innerHeight - listV - footerHeight - 2
	if listHeight < 0 {
		listHeight = 0
	}

	m.albumList.SetSize(listWidth, listHeight)
	m.songList.SetSize(listWidth, listHeight)
}

func (m mainModel) renderHeader(title string, innerWidth int) string {
	// No song playing
	styledTitle := titleStyle.Render(title)
	if m.nowPlaying == "" {
		return styledTitle
	}

	// While song playing
	var playingText string
	if m.isPaused {
		playingText = playingStyle.Render(fmt.Sprintf("⏸  %s", m.nowPlaying))
	} else {
		playingText = playingStyle.Render(fmt.Sprintf("▶️ %s", m.nowPlaying))
	}

	spaces := innerWidth - lipgloss.Width(styledTitle) - lipgloss.Width(playingText) - 2
	if spaces < 2 {
		spaces = 2
	}
	return styledTitle + strings.Repeat(" ", spaces) + playingText
}

func (m mainModel) renderHelp(pairs [][2]string) string {
	if len(pairs) == 0 {
		return ""
	}
	var parts []string
	keyStyle := m.albumList.Help.Styles.ShortKey
	descStyle := m.albumList.Help.Styles.ShortDesc
	sepStyle := m.albumList.Help.Styles.ShortSeparator

	for _, pair := range pairs {
		parts = append(parts, keyStyle.Render(pair[0])+descStyle.Render(pair[1]))
	}
	return strings.Join(parts, sepStyle.Render(" • "))
}

type albumItem struct {
	album client.Album
}

func (i albumItem) Title() string { return i.album.Title }
func (i albumItem) Description() string {
	return fmt.Sprintf("Platform: %s | Type: %s | Year: %s",
		strings.TrimSpace(i.album.Platform),
		strings.TrimSpace(i.album.Type),
		strings.TrimSpace(i.album.Year))
}
func (i albumItem) FilterValue() string { return i.album.Title }

type songItem struct {
	song client.Song
}

func (i songItem) Title() string { return i.song.Title }
func (i songItem) Description() string {
	desc := fmt.Sprintf("Duration: %s | MP3: %s", i.song.Duration, i.song.MP3Size)
	if i.song.FLACSize != "" {
		desc += " | FLAC: " + i.song.FLACSize
	}
	return desc
}
func (i songItem) FilterValue() string { return i.song.Title }

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.waitForDownload())
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If a filter input is active in the album view, let the list component handle all keys
		if m.state == stateSongList && m.songList.FilterState() == list.Filtering {
			// We only intercept Ctrl+C to allow quitting the application even while filtering
			if msg.String() == "ctrl+c" {
				m.stopPlayback()
				return m, tea.Quit
			}
			break
		}

		switch msg.String() {
		case "tab":
			if m.config.Format == "flac" {
				m.config.Format = "mp3"
			} else {
				m.config.Format = "flac"
			}
			_ = m.config.Save()
			m.downloader.Config.Format = m.config.Format
			return m, nil
		case "ctrl+c", "q":
			m.stopPlayback()
			return m, tea.Quit
		case "esc", "backspace":
			switch m.state {
			case stateAlbumList:
				m.state = stateSearch
				m.searchInput.Focus()
				return m, nil
			case stateSongList:
				// If a filter is currently applied, clear it
				if m.songList.FilterValue() != "" {
					m.songList.ResetFilter()
					return m, nil
				}
				m.state = stateAlbumList
				return m, nil
			}
		case "p":
			if m.nowPlaying != "" && m.state != stateSearch {
				if m.isPaused {
					_ = resumeProcess(m.player)
					m.isPaused = false
				} else {
					_ = pauseProcess(m.player)
					m.isPaused = true
				}
				return m, nil
			}
		case "s":
			if m.nowPlaying != "" && m.state != stateSearch {
				m.stopPlayback()
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcSizes()
	case searchResultMsg:
		m.state = stateAlbumList
		var items []list.Item
		for _, a := range msg.albums {
			items = append(items, albumItem{album: a})
		}

		m.albumList.SetItems(items)
		m.albumList.Select(0)
		m.recalcSizes()
		return m, nil

	case albumDetailsMsg:
		m.state = stateSongList
		m.currentAlbum = msg.details
		var items []list.Item
		for _, s := range msg.details.Songs {
			items = append(items, songItem{song: s})
		}

		m.songList.SetItems(items)
		m.songList.ResetFilter()
		m.songList.Select(0)
		m.songList.Title = msg.details.Title
		m.recalcSizes()
		return m, nil

	case playbackStartedMsg:
		m.player = msg.cmd
		m.nowPlaying = msg.songTitle
		m.recalcSizes()
		return m, m.waitForPlayback(msg.cmd)

	case playbackFinishedMsg:
		m.nowPlaying = ""
		m.recalcSizes()
		return m, nil

	case downloader.DownloadProgress:
		m.downloadInfo = msg
		m.showProgress = true
		if msg.Completed == msg.Total && msg.Total > 0 {
			m.queueCount--
			if m.queueCount <= 0 {
				m.queueCount = 0
				cmds = append(cmds, m.hideProgressCmd())
			}
		}

		pct := float64(msg.Completed) / float64(msg.Total)
		cmds = append(cmds, m.progressBar.SetPercent(pct))
		cmds = append(cmds, m.waitForDownload())
		m.recalcSizes()
		return m, tea.Batch(cmds...)

	case progress.FrameMsg:
		newModel, cmd := m.progressBar.Update(msg)
		if pm, ok := newModel.(progress.Model); ok {
			m.progressBar = pm
		}
		return m, cmd

	case hideProgressMsg:
		m.showProgress = false
		m.recalcSizes()
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.state = stateSearch
		return m, nil
	}

	switch m.state {
	case stateSearch:
		m.searchInput, cmd = m.searchInput.Update(msg)
		cmds = append(cmds, cmd)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
			query := m.searchInput.Value()
			if query != "" {
				m.state = stateLoading
				m.loadingMsg = "Searching..."
				return m, m.performSearch(query)
			}
		}

	case stateAlbumList:
		m.albumList, cmd = m.albumList.Update(msg)
		cmds = append(cmds, cmd)
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			keyStr := strings.ToLower(keyMsg.String())
			switch keyStr {
			case "enter":
				i, ok := m.albumList.SelectedItem().(albumItem)
				if ok {
					m.state = stateLoading
					m.loadingMsg = "Loading album..."
					return m, m.loadAlbum(i.album.URL)
				}
			case "d":
				i, ok := m.albumList.SelectedItem().(albumItem)
				if ok {
					m.queueCount++
					m.taskChan <- downloadTask{url: i.album.URL, isSong: false}
					cmds = append(cmds, m.albumList.NewStatusMessage(fmt.Sprintf("Added to queue: %s", i.album.Title)))
				}
			}
		}

	case stateSongList:
		wasFiltering := m.songList.FilterState() == list.Filtering
		m.songList, cmd = m.songList.Update(msg)
		cmds = append(cmds, cmd)
		if keyMsg, ok := msg.(tea.KeyMsg); ok && !wasFiltering {
			keyStr := strings.ToLower(keyMsg.String())
			switch keyStr {
			case "enter":
				i, ok := m.songList.SelectedItem().(songItem)
				if ok {
					m.stopPlayback()
					return m, m.playSong(i.song)
				}
			case "d":
				i, ok := m.songList.SelectedItem().(songItem)
				if ok {
					m.queueCount++
					m.taskChan <- downloadTask{song: i.song, albumTitle: m.currentAlbum.Title, isSong: true}
					cmds = append(cmds, m.songList.NewStatusMessage(fmt.Sprintf("Added to queue: %s", i.song.Title)))
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m mainModel) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	h, v := appStyle.GetFrameSize()
	innerWidth := m.width - h
	innerHeight := m.height - v

	if m.err != nil {
		errorView := fmt.Sprintf("%s\n\nPress Esc to go back", errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		content := lipgloss.Place(innerWidth, innerHeight, lipgloss.Center, lipgloss.Center, errorView)
		return appStyle.Width(innerWidth).Height(innerHeight).Render(content)
	}
	if m.state == stateLoading {
		loadingText := loadingStyle.Render(m.loadingMsg)
		content := lipgloss.Place(innerWidth, innerHeight, lipgloss.Center, lipgloss.Center, loadingText)
		return appStyle.Width(innerWidth).Height(innerHeight).Render(content)
	}

	var mainView string
	var helpPairs1 [][2]string
	var helpPairs2 [][2]string
	switch m.state {
	case stateSearch:
		header := m.renderHeader("KHInsider Explorer", innerWidth)
		mainView = fmt.Sprintf(
			"\n%s\n\n  %s",
			header,
			m.searchInput.View(),
		)
		helpPairs1 = [][2]string{
			{"Enter ↵", " search"},
			{"Ctrl+C", " quit app"},
		}
		helpPairs2 = [][2]string{
			{"Tab ⇆", fmt.Sprintf(" format [%s]", strings.ToUpper(m.config.Format))},
		}
	case stateAlbumList:
		header := m.renderHeader("Search Results", innerWidth)
		var body string
		if len(m.albumList.Items()) == 0 {
			body = "\n" + noItemsStyle.Render("No OSTs found.")
		} else {
			body = m.albumList.View()
		}
		mainView = fmt.Sprintf("\n%s\n%s", header, body)
		helpPairs1 = [][2]string{
			{"Enter ↵", " open"},
			{"D", " download album"},
			{"Esc", " back"},
		}
		helpPairs2 = [][2]string{
			{"Tab ⇆", fmt.Sprintf(" format [%s]", strings.ToUpper(m.config.Format))},
		}
	case stateSongList:
		titleText := m.currentAlbum.Title
		if titleText == "" {
			titleText = "Album Songs"
		}

		header := m.renderHeader(titleText, innerWidth)
		mainView = fmt.Sprintf("\n%s\n%s", header, m.songList.View())
		helpPairs1 = [][2]string{
			{"Enter ↵", " play"},
			{"D", " download song"},
		}

		// Show lower Esc hint only while filter is not active
		if m.songList.FilterValue() == "" {
			helpPairs1 = append(helpPairs1, [2]string{"Esc", " back"})
		}
		helpPairs2 = [][2]string{
			{"Tab ⇆", fmt.Sprintf(" format [%s]", strings.ToUpper(m.config.Format))},
		}
	}

	if m.state == stateSongList && m.songList.FilterState() == list.Filtering {
		helpPairs1 = nil
		helpPairs2 = nil
	} else if m.nowPlaying != "" {
		if m.isPaused {
			helpPairs2 = append(helpPairs2, [2]string{"P", " play"}, [2]string{"S", " stop"})
		} else {
			helpPairs2 = append(helpPairs2, [2]string{"P", " pause"}, [2]string{"S", " stop"})
		}
	}

	helpText1 := m.renderHelp(helpPairs1)
	helpText2 := m.renderHelp(helpPairs2)

	footer := ""
	if helpText1 != "" {
		footer += "\n" + helpStyle.Render(helpText1)
	} else {
		footer += "\n"
	}
	if helpText2 != "" {
		footer += "\n" + helpStyle.Render(helpText2)
	} else {
		footer += "\n"
	}

	// Download Progress Bar (Always occupies exactly the bottom 2 lines of the footer)
	var progressStatus string
	var progressBarView string
	if m.showProgress {
		status := fmt.Sprintf("Downloading %s (%d/%d)", m.downloadInfo.Current, m.downloadInfo.Completed, m.downloadInfo.Total)
		if m.queueCount > 1 {
			status += fmt.Sprintf(" [%d in queue]", m.queueCount-1)
		}
		if m.downloadInfo.Completed == m.downloadInfo.Total && m.queueCount == 0 {
			status = "All downloads complete!"
		}
		progressStatus = "  " + downloadStyle.Render(status)
		progressBarView = "  " + m.progressBar.View()
	}

	footer += "\n\n" + progressStatus + "\n" + progressBarView

	content := mainView + footer
	contentHeight := lipgloss.Height(content)
	extraLines := innerHeight - contentHeight

	if extraLines > 0 {
		mainView += strings.Repeat("\n", extraLines)
		content = mainView + footer
	}

	return appStyle.Width(innerWidth).Height(innerHeight).Render(content)
}

// Commands
type searchResultMsg struct{ albums []client.Album }
type albumDetailsMsg struct{ details client.AlbumDetails }
type playbackStartedMsg struct {
	cmd       *exec.Cmd
	songTitle string
}
type playbackFinishedMsg struct{}
type hideProgressMsg struct{}
type errorMsg struct{ err error }

func (m mainModel) performSearch(query string) tea.Cmd {
	return func() tea.Msg {
		albums, err := m.client.Search(query)
		if err != nil {
			return errorMsg{err}
		}
		return searchResultMsg{albums}
	}
}

func (m mainModel) loadAlbum(url string) tea.Cmd {
	return func() tea.Msg {
		details, err := m.client.GetAlbumDetails(url)
		if err != nil {
			return errorMsg{err}
		}
		return albumDetailsMsg{details}
	}
}

func (m *mainModel) stopPlayback() {
	if m.player != nil && m.player.Process != nil {
		_ = m.player.Process.Kill()
		m.player = nil
		m.nowPlaying = ""
		m.isPaused = false
	}
}

func (m mainModel) playSong(song client.Song) tea.Cmd {
	return func() tea.Msg {
		player, args := m.detectPlayer()
		if player == "" {
			return errorMsg{fmt.Errorf("no media player found. Please install mpv, ffplay, or vlc")}
		}

		url, err := m.client.GetDownloadURL(song.URL, m.config.Format)
		if err != nil || url == "" {
			if err == nil {
				err = fmt.Errorf("could not find download link for %s", song.Title)
			}
			return errorMsg{err}
		}

		args = append(args, url)
		cmd := exec.Command(player, args...)
		err = cmd.Start()
		if err != nil {
			return errorMsg{fmt.Errorf("failed to start %s: %v", player, err)}
		}

		return playbackStartedMsg{cmd, song.Title}
	}
}

func (m mainModel) detectPlayer() (string, []string) {
	// 1. Try user configured player
	if m.config.Player != "" && m.config.Player != "mpv" {
		if _, err := exec.LookPath(m.config.Player); err == nil {
			return m.config.Player, []string{}
		}
	}

	// 2. Try mpv
	if _, err := exec.LookPath("mpv"); err == nil {
		args := []string{"--no-video", "--volume=100", "--user-agent=" + m.client.UserAgent, "--referrer=https://downloads.khinsider.com/"}
		if runtime.GOOS == "linux" {
			args = append(args, "--ao=pulse,alsa")
		}
		return "mpv", args
	}

	// 3. Try ffplay
	if _, err := exec.LookPath("ffplay"); err == nil {
		return "ffplay", []string{"-nodisp", "-autoexit", "-volume", "100", "-user_agent", m.client.UserAgent}
	}

	// 4. Try vlc (common on Windows)
	vlcPath := "vlc"
	if runtime.GOOS == "windows" {
		vlcPath = `C:\Program Files\VideoLAN\VLC\vlc.exe`
	}
	if _, err := exec.LookPath(vlcPath); err == nil {
		return vlcPath, []string{"--intf", "dummy", "--play-and-exit", "--no-video"}
	}

	return "", nil
}

func (m mainModel) waitForPlayback(cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		_ = cmd.Wait()
		return playbackFinishedMsg{}
	}
}

func (m mainModel) waitForDownload() tea.Cmd {
	return func() tea.Msg {
		return <-m.downloadChan
	}
}

func (m mainModel) hideProgressCmd() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		return hideProgressMsg{}
	}
}

func Start(cfg config.Config) error {
	p := tea.NewProgram(NewMainModel(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
