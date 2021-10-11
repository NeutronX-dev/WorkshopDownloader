package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/parnurzeal/gorequest"
	"github.com/tidwall/gjson"
)

type WorkshopDownloader struct {
	folder      string
	updateLabel *widget.Label
	pathLabel   *widget.Label
}

func (downloader *WorkshopDownloader) SetFolder(arg string) {
	if arg[0:7] == "file://" {
		arg = arg[7:]
	}
	downloader.folder = arg
	downloader.pathLabel.SetText("Path: " + arg)
}

func (downloader *WorkshopDownloader) FolderSet() bool {
	return downloader.folder != ""
}

func (downloader *WorkshopDownloader) SetUpdateLabel(arg *widget.Label) {
	downloader.updateLabel = arg
}

func (downloader *WorkshopDownloader) SetPathLabel(arg *widget.Label) {
	downloader.pathLabel = arg
}

func (downloader *WorkshopDownloader) DownloadFile(idURL, url string) error {
	file, err := os.Create(idURL + ".zip")
	if err != nil {
		return err
	}
	defer file.Close()

	resp, err := http.Get(url)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func (downloader *WorkshopDownloader) Unzip(id string) ([]string, error) {

	defaultWorkingDIrectory, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer os.Chdir(defaultWorkingDIrectory)

	var src string = id + ".zip"
	var dest string = path.Join(downloader.folder + "\\" + id)
	var filenames []string
	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {
		os.Chdir(downloader.folder)
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}

func (downloader *WorkshopDownloader) UpdateStatus(status string) {
	downloader.updateLabel.SetText("Status: " + status)
}

func (downloader *WorkshopDownloader) HandleDownload(_url string) error {
	url, err := url.ParseRequestURI(_url)
	if err != nil {
		return fmt.Errorf("e: Invalid URL")
	}

	idUrl := url.Query().Get("id")
	if idUrl == "" {
		return fmt.Errorf("e: Invalid URL")
	}
	downloader.UpdateStatus("Checking if Avaliable")
	request := gorequest.New()
	resp, body, _ := request.Post("https://backend-02-prd.steamworkshopdownloader.io/api/download/request").
		Set("Content-Type", "application/json").
		Send(`{"publishedFileId":` + idUrl + `, "collectionId":0, "extract":true, "hidden":false, "direct":false, "autodownload":true}`).
		End()

	if resp.StatusCode != 200 {
		return fmt.Errorf("e: Unavaliable or server is Down")
	} else {
		downloader.UpdateStatus("Avaliable")
	}

	uid := gjson.Get(body, "uuid").String()
	var readyFile = false

	for i := 0; i < 10; i++ {
		_, body, _ := request.Post("https://backend-02-prd.steamworkshopdownloader.io/api/download/status").
			Set("Content-Type", "application/json").
			Send(`{"uuids": ["` + uid + `"]}`).
			End()

		downloader.UpdateStatus(strings.ToUpper(gjson.Get(body, uid+".status").String()))

		if strings.Contains(body, "prepared") {
			readyFile = true
			downloader.UpdateStatus("Downloading")
			break
		}
		time.Sleep(2 * time.Second)
	}

	if readyFile {
		err := downloader.DownloadFile(idUrl, "https://backend-02-prd.steamworkshopdownloader.io/api/download/transmit?uuid="+uid)
		if err != nil {
			return err
		} else {
			downloader.UpdateStatus("Downloaded, Trying to Unzip")
			_, err := downloader.Unzip(idUrl)
			if err != nil {
				return err
			}
			downloader.UpdateStatus("Unzipped")
			err = os.Remove(idUrl + ".zip")
			if err != nil {
				return err
			}
			downloader.UpdateStatus("Removed remaining ZIP.")

			return nil
		}

	} else {
		return fmt.Errorf("e: FAIL THE SERVER IS BUSY")
	}
}

func FolderOpenHandler(win fyne.Window, downloader *WorkshopDownloader) func(fyne.ListableURI, error) {
	return func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		if list == nil {
			return
		}
		downloader.SetFolder(list.String())
	}
}

func main() {

	var Downloader WorkshopDownloader

	Application := app.New()
	win := Application.NewWindow("Workshop Downloader")
	win.Resize(fyne.NewSize(511.53848, 296.9231))

	URLInput := widget.NewEntry()
	URLInput.PlaceHolder = "URL"

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Download Path", Widget: widget.NewButton("Choose", func() {
				dialog.ShowFolderOpen(FolderOpenHandler(win, &Downloader), win)
			})}},
		OnSubmit: func() {
			if URLInput.Text != "" {
				if Downloader.FolderSet() {
					err := Downloader.HandleDownload(URLInput.Text)
					if err != nil {
						dialog.ShowError(err, win)
						Downloader.UpdateStatus(err.Error())
						return
					}

				} else {
					dialog.ShowError(fmt.Errorf("e: Download Folder not Specified"), win)
				}
			} else {
				dialog.ShowError(fmt.Errorf("e: Invalid URL"), win)
			}
		},
	}
	form.Append("URL", URLInput)

	updateLabel := widget.NewLabel("Status: Not Started")
	pathLabel := widget.NewLabel("Path: Not Specified")
	Downloader.SetPathLabel(pathLabel)
	Downloader.SetUpdateLabel(updateLabel)

	MainContainer := container.NewVSplit(form, container.NewCenter(container.NewAdaptiveGrid(1, container.NewCenter(updateLabel), container.NewCenter(pathLabel))))

	win.SetContent(MainContainer)
	win.ShowAndRun()
}
