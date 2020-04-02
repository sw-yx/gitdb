package gitdb

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gorilla/mux"
)

//add to your package: //go:generate gitdb emded-ui -o ./ui_gitdb.go

var serverRoot = "./"
var ui *gui

//UI provides an API to run UI server from outside this package
func UI() GUI {
	if ui == nil {
		ui = &gui{}
		ui.files = make(map[string]string)
	}
	return ui
}

func (g *gitdb) startUI() {
	go UI().serve(g)
	//listen for shutdown event
	go func() {
		for {
			select {
			case <-g.shutdown:
				if ui.server != nil {
					ui.server.Shutdown(context.TODO())
				}
				logTest("shutting down UI server")
				return
			}
		}
	}()
}

//GUI interface
type GUI interface {
	serve(GitDb)
	Embed(name, src string)
}

type gui struct {
	server *http.Server
	files  map[string]string
}

var nextDatasetRefresh time.Time

func (e *gui) serve(db GitDb) {

	_, filename, _, ok := runtime.Caller(0)
	if ok {
		serverRoot = path.Dir(filename)
	}

	uh := &uiHandler{}
	eps := uh.getEndpoints()
	router := mux.NewRouter()
	for _, ep := range eps {
		router.HandleFunc(ep.Path, ep.Handler)
	}

	port := db.Config().UIPort
	if port == 0 {
		port = defaultUIPort
	}

	addr := fmt.Sprintf("localhost:%d", port)
	log(fmt.Sprintf("Server Root : %q", path.Dir(filename)))
	log("GitDB GUI will run at http://" + addr)

	//refresh dataset after 1 minute
	router.Use(func(h http.Handler) http.Handler {

		if nextDatasetRefresh.IsZero() || nextDatasetRefresh.Before(time.Now()) {
			uh.datasets = loadDatasets(db.Config())
			nextDatasetRefresh = time.Now().Add(time.Minute * 1)
		}

		return h
	})

	e.server = &http.Server{Addr: addr, Handler: router}

	if err := e.server.ListenAndServe(); err != nil {
		logError(err.Error())
	}
}

func (e *gui) Embed(name, src string) {
	e.files[name] = src
}

func (e *gui) has(name string) bool {
	_, ok := e.files[name]
	return ok
}

func (e *gui) get(name string) string {
	content := e.files[name]
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		logError(err.Error())
		return ""
	}

	return string(decoded)
}

//endpoint maps a path to a http handler
type endpoint struct {
	Path    string
	Handler func(w http.ResponseWriter, r *http.Request)
}

//uiHandler provides all the http handlers for the UI
type uiHandler struct {
	datasets []*dataset
}

func (u *uiHandler) getEndpoints() []*endpoint {
	return []*endpoint{
		{"/css/app.css", u.appCSS},
		{"/js/app.js", u.appJS},
		{"/", u.overview},
		{"/errors/{dataset}", u.viewErrors},
		{"/list/{dataset}", u.list},
		{"/view/{dataset}", u.view},
		{"/view/{dataset}/b{b}/r{r}", u.view},
	}
}

func (u *uiHandler) render(w http.ResponseWriter, data interface{}, templates ...string) {

	parseFiles := false
	fTemplates := make([]string, len(templates))
	for i, template := range templates {
		fTemplates[i] = fq(template)
		if !ui.has(fTemplates[i]) {
			parseFiles = true
		}
	}

	var t *template.Template
	var err error
	if parseFiles {
		t, err = template.ParseFiles(fTemplates...)
		if err != nil {
			logError(err.Error())
		}
	} else {
		t = template.New("overview")
		for _, template := range fTemplates {
			logTest("Reading EMBEDDED file - " + template)
			t, err = t.Parse(ui.get(template))
			if err != nil {
				logError(err.Error())
			}
		}
	}

	t.Execute(w, data)
}

func (u *uiHandler) appCSS(w http.ResponseWriter, r *http.Request) {
	src := readView("static/css/app.css")
	w.Header().Set("Content-Type", "text/css")
	w.Write([]byte(src))
}

func (u *uiHandler) appJS(w http.ResponseWriter, r *http.Request) {
	src := readView("static/js/app.js")
	w.Header().Set("Content-Type", "text/javascript")
	w.Write([]byte(src))
}

func (u *uiHandler) overview(w http.ResponseWriter, r *http.Request) {

	viewModel := &overviewViewModel{}
	viewModel.Title = "Overview"
	viewModel.DataSets = u.datasets

	u.render(w, viewModel, "static/index.html", "static/sidebar.html")
}

func (u *uiHandler) list(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	viewDs := vars["dataset"]

	dataset := u.findDataset(viewDs)
	if dataset != nil {
		block := dataset.Blocks[0]
		table := block.table()

		viewModel := &listDataSetViewModel{DataSet: dataset, Table: table}
		viewModel.DataSets = u.datasets

		u.render(w, viewModel, "static/list.html", "static/sidebar.html")
	} else {
		w.Write([]byte("Dataset (" + viewDs + ") does not exist"))
	}
}

func (u *uiHandler) view(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	viewModel := &viewDataSetViewModel{}
	if viewModel.Pager == nil {
		viewModel.Pager = &pager{}
	}

	viewDs := vars["dataset"]
	blockFlag := vars["b"]
	recordFlag := vars["r"]

	if blockFlag == "" && recordFlag == "" {
		viewModel.Pager.reset()
	} else {
		viewModel.Pager.set(blockFlag, recordFlag)
	}

	dataset := u.findDataset(viewDs)
	if dataset != nil {
		block := dataset.Blocks[viewModel.Pager.blockPage]

		viewModel.Pager.totalBlocks = dataset.BlockCount()
		viewModel.Pager.totalRecords = block.RecordCount()

		content := "No record found"
		if block.RecordCount() > viewModel.Pager.recordPage {
			content = block.Records[viewModel.Pager.recordPage].Content
		}

		viewModel.DataSet = dataset
		viewModel.Content = content
		viewModel.DataSets = u.datasets

		u.render(w, viewModel, "static/view.html", "static/sidebar.html")
	} else {
		w.Write([]byte("Dataset (" + viewDs + ") does not exist"))
	}
}

func (u *uiHandler) viewErrors(w http.ResponseWriter, r *http.Request) {

	vars := mux.Vars(r)
	viewDs := vars["dataset"]

	dataset := u.findDataset(viewDs)
	if dataset != nil {
		viewModel := &errorsViewModel{DataSet: dataset}
		viewModel.Title = "Errors"
		viewModel.DataSets = u.datasets

		u.render(w, viewModel, "static/errors.html", "static/sidebar.html")
	} else {
		w.Write([]byte("Dataset (" + viewDs + ") does not exist"))
	}
}

func (u *uiHandler) findDataset(name string) *dataset {
	for _, ds := range u.datasets {
		if ds.Name == name {
			return ds
		}
	}
	return nil
}

func fq(path string) string {
	return filepath.Join(serverRoot, path)
}

func readView(fileName string) string {
	fqFilename := fq(fileName)
	if ui.has(fqFilename) {
		return ui.get(fqFilename)
	}

	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		logError(err.Error())
		return ""
	}

	return string(data)
}
