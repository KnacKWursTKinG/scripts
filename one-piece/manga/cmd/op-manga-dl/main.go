package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"op-manga-dl/internal/scraper"
	"op-manga-dl/internal/utils"

	"github.com/lmittmann/tint"
)

var (
	c *Config
)

func main() {
	c = NewConfig()
	parseFlags()

	for true {
		downloadAllChapters()
		sleep()
	}
}

func downloadAllChapters() {
	slog.Debug("download all chapters")

	ml, err := scraper.ParseMangaList()
	if err != nil {
		slog.Error("Fetch & parse manga list", "err", err.Error())
		return
	}

    currentDownloads := 0
	for _, chapter := range ml.Chapters {
		if chapter.Pages == 0 {
			continue
		}

		arc, i := ml.GetArc(chapter.ArcId)
		if arc == nil {
			slog.Error(fmt.Sprintf(
				"Arc for %s with the id %d not found! (This should never happen)",
				chapter.Name, chapter.ArcId,
			))
		}

		path := filepath.Join(
            c.Download.Dst,
			fmt.Sprintf("%03d %s", len(ml.Arcs)-i, arc.Name),
			fmt.Sprintf("%04d %s", chapter.Number, chapter.Name),
		)

		_, err := os.Stat(path + ".pdf")
		if err == nil {
            // File (pdf) already exists, continue to next chapter
            continue
        }

        // Make directory where the pages will be stored (ignore errors if already exists)
        _ = os.MkdirAll(path, 0755)

        currentDownloads += 1
        downloadChapter(chapter, path)

        // Handle the short and long download delay based on limit
        duration := time.Minute * time.Duration(c.Download.Delay)
        if currentDownloads >= c.Download.LimitPerDay {
            duration = time.Minute * time.Duration(c.Download.LongDelay)
            currentDownloads = 0
        }

        slog.Debug("download delay", "duration", duration, "currentDownloads", currentDownloads)
        time.Sleep(duration)
	}
}

func downloadChapter(chapter scraper.MangaList_Chapter, path string) {
    slog.Info(fmt.Sprintf("Download chapter \"%s\" with %d pages.", chapter.Name, chapter.Pages))

	// download jpg/png from dURL - scrape the same script section like before
	chapterData, err := scraper.ParseChapter(chapter.Href)
	if err != nil {
		slog.Error("Parsing chapter failed!", "chapter.Href", chapter.Href, "err", err.Error())
		return
	}

	pages := make([]string, len(chapterData.Chapter.Pages))
	for i, page := range chapterData.Chapter.Pages {
		slog.Debug("downloading page", "page.Url", page.Url)
		r, err := http.Get(page.Url)
		if err != nil {
			slog.Error("Downloading page failed!", "page", i+1, "err", err.Error())
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Error("Read all body data failed!", "page", i+1, "err", err.Error())
			return
		}
		if len(data) == 0 {
			slog.Error("No data!", "page", i+1)
			return
		}
		e, _ := utils.GetExtension(page.Type)
		p := filepath.Join(path, fmt.Sprintf("%02d.%s", i+1, e))
		err = os.WriteFile(p, data, 0644)
		if err != nil {
			slog.Error(fmt.Sprintf("Write file \"%s\" failed!", p), "err", err.Error())
			return
		}
		pages[i] = p
	}

	if err := utils.ConvertImagesToPDF(path, pages...); err != nil {
		slog.Error("Convert pages to pdf failed!", "err", err.Error())
	}
}

func sleep() {
	for true {
		now := time.Now()

		var day int
		if now.Hour() < c.Update.Hour {
			day = now.Day()
		} else {
			day = now.Day() + 1
		}
		next := time.Date(now.Year(), now.Month(), day, c.Update.Hour, 0, 0, 0, time.Local)

		duration := next.Sub(now)
		slog.Debug("sleep until next update day", "duration", duration)

		time.Sleep(duration)

		if time.Now().Weekday() == c.Update.Weekday {
			slog.Debug("running new update now...")
			break
		}
	}
}

func parseFlags() {
	flag.BoolVar(&c.Debug, "debug", c.Debug, "Enable debugging")

	flag.IntVar(
		&c.Download.Delay,
		"delay",
		c.Download.Delay,
		"Set delay in minutes between downloads",
	)

	flag.IntVar(
		&c.Download.LongDelay,
		"long-delay",
		c.Download.LongDelay,
		"Set long delay in minutes if download limit was reached",
	)

	flag.StringVar(
		&c.Download.Dst,
		"dst",
		c.Download.Dst,
		"Set destination path for downloads",
	)

	flag.IntVar(
		&c.Download.LimitPerDay,
		"limit",
		c.Download.LimitPerDay,
		"Download limit (per day)",
	)

	weekday := flag.Int("update-on-day", int(c.Update.Weekday),
		"Weekday (0-6) for update the anime list")

	hour := flag.Int("update-hour", c.Update.Hour,
		"Hour (0-23) for anime list update")

	flag.Parse()

	if *weekday >= 0 && *weekday <= 6 {
		c.Update.Weekday = time.Weekday(*weekday)
	}

	if *hour >= 0 && *hour <= 23 {
		c.Update.Hour = *hour
	}

    options := &tint.Options{
        TimeFormat: time.Kitchen,
		Level: slog.LevelInfo,
    }

	if c.Debug {
        options.Level = slog.LevelDebug
	}

    slog.SetDefault(slog.New(tint.NewHandler(os.Stderr, options)))
}
